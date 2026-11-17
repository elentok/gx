package chatdrafts

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/server/imagestore"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

// imagesName is the folder of a draft's images, inside the draft's own folder,
// so Delete removes them with the draft.
const imagesName = "images"

// startedImagesDir is the folder Start copies a draft's images to, under the
// chat-drafts root but outside any draft, so they outlive the draft.
const startedImagesDir = "started-images"

// ImagesDir is the folder a draft's images live in.
func (s *Store) ImagesDir(id string) string { return filepath.Join(s.dir(id), imagesName) }

func goneImage() wire.SessionImageResult {
	return imagestore.Refusal(ops.ReasonDraftGone, "draft is gone")
}

// AddImage stores one image in the draft's folder and lists its id on the
// record. It changes the image list only: the text revision stays, so a text
// save in flight never conflicts with it. A missing draft is a refused result.
func (s *Store) AddImage(id string, data []byte) (wire.SessionImageResult, error) {
	if !idPattern.MatchString(id) {
		return goneImage(), nil
	}
	unlock := s.lock(id)
	defer unlock()
	d, err := s.Get(id)
	if errors.Is(err, ErrNotFound) {
		return goneImage(), nil
	}
	if err != nil {
		return wire.SessionImageResult{}, err
	}
	if len(d.Images) >= wire.MaxPromptImages {
		return imagestore.Refusal(wire.ReasonImageTooMany, fmt.Sprintf("a draft holds at most %d images", wire.MaxPromptImages)), nil
	}
	res, err := imagestore.Save(s.ImagesDir(id), data)
	if res.Refused || err != nil {
		return res, err
	}
	d.Images = append(d.Images, res.ID)
	if err := s.write(d); err != nil {
		_, _, _ = imagestore.Remove(s.ImagesDir(id), res.ID)
		return wire.SessionImageResult{}, err
	}
	return res, nil
}

// RemoveImage deletes one image file and drops its id from the record, leaving
// the text revision alone.
func (s *Store) RemoveImage(id, image string) (wire.SessionImageResult, error) {
	if !idPattern.MatchString(id) {
		return goneImage(), nil
	}
	unlock := s.lock(id)
	defer unlock()
	d, err := s.Get(id)
	if errors.Is(err, ErrNotFound) {
		return goneImage(), nil
	}
	if err != nil {
		return wire.SessionImageResult{}, err
	}
	i := slices.Index(d.Images, image)
	if i < 0 {
		return imagestore.Refusal(wire.ReasonImageNotFound, fmt.Sprintf("no image %s in draft %s", image, id)), nil
	}
	name, _, err := imagestore.Remove(s.ImagesDir(id), image)
	if err != nil {
		return wire.SessionImageResult{}, err
	}
	d.Images = slices.Delete(d.Images, i, i+1)
	if err := s.write(d); err != nil {
		return wire.SessionImageResult{}, err
	}
	return wire.SessionImageResult{ID: image, Name: name}, nil
}

// CopyImages copies the draft's images to <root>/started-images/<id> and
// returns their absolute paths in the draft's order. A listed image whose file
// is missing is skipped. The caller holds the draft's lock.
func (s *Store) CopyImages(d *ops.ChatDraft) ([]string, error) {
	var paths []string
	dest := filepath.Join(s.root, startedImagesDir, d.ID)
	for _, id := range d.Images {
		src, ok := imagestore.Find(s.ImagesDir(d.ID), id)
		if !ok {
			continue
		}
		b, err := os.ReadFile(src)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(dest, 0o700); err != nil {
			return nil, err
		}
		to := filepath.Join(dest, filepath.Base(src))
		if err := os.WriteFile(to, b, 0o600); err != nil {
			return nil, err
		}
		paths = append(paths, to)
	}
	return paths, nil
}

// withImages is the brief with an "Images:" list of paths after it.
func withImages(brief string, paths []string) string {
	if len(paths) == 0 {
		return brief
	}
	return brief + "\n\nImages:\n" + strings.Join(paths, "\n")
}

// imageUpload serves POST /v1/agents/drafts/images?id=<draft>: the body is one
// image's raw bytes, capped like a session image upload.
func (c *ChatDrafts) imageUpload(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, imagestore.MaxBytes+64<<10))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		wire.WriteJSON(w, imagestore.TooLarge())
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := c.store.AddImage(r.URL.Query().Get("id"), body)
	if err != nil {
		serverError(w, err)
		return
	}
	wire.WriteJSON(w, res)
}

// imageRemove serves POST /v1/agents/drafts/images/delete.
func (c *ChatDrafts) imageRemove(w http.ResponseWriter, r *http.Request) {
	req, ok := decode[wire.DraftImageRequest](w, r)
	if !ok {
		return
	}
	res, err := c.store.RemoveImage(req.ID, req.Image)
	if err != nil {
		serverError(w, err)
		return
	}
	wire.WriteJSON(w, res)
}

// imageServe serves GET /v1/agents/drafts/images?id=<draft>&image=<image>.
// Exists validates the draft id before it names a folder.
func (c *ChatDrafts) imageServe(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !c.store.Exists(id) {
		http.NotFound(w, r)
		return
	}
	imagestore.Serve(w, r, c.store.ImagesDir(id), r.URL.Query().Get("image"))
}

// startedImagesMaxAge is how long a started chat's image copies are kept. The
// chat's prompt names them by path, so they must outlive the draft; nothing
// ties them to the session's removal, so New prunes by age instead.
const startedImagesMaxAge = 30 * 24 * time.Hour

// pruneStartedImages removes the started-images folders last modified more
// than maxAge ago. It is best effort: a folder it cannot remove stays.
func (s *Store) pruneStartedImages(maxAge time.Duration) {
	base := filepath.Join(s.root, startedImagesDir)
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	cutoff := s.now().Add(-maxAge)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !e.IsDir() || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(base, e.Name()))
	}
}
