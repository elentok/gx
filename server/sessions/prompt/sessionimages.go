package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"

	"github.com/elentok/gx/server/imagestore"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
)

const (
	// imageBodyCap is the route's own body cap, a little above imagestore.MaxBytes so
	// an image of exactly that size fits and one byte more is read in full
	// and refused with a reason instead of cut off.
	imageBodyCap = imagestore.MaxBytes + 64<<10
	// imagesDir is the folder of a session's Prompt images, inside its state dir.
	imagesDir = chat.ImagesDir
)

// imageUploadHandler serves POST /v1/sessions/images?id=<session>: the body is
// one image's raw bytes.
func (p *Prompt) imageUploadHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, imageBodyCap))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		wire.WriteJSON(w, imagestore.TooLarge())
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := p.saveSessionImage(r.URL.Query().Get("id"), body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

// saveSessionImage stores one Prompt image in the session's images folder
// under the session gates.
func (p *Prompt) saveSessionImage(sessionID string, data []byte) (wire.SessionImageResult, error) {
	dir, gate, err := p.imageSessionDir(sessionID)
	if gate.Refused || err != nil {
		return gate, err
	}
	return imagestore.Save(filepath.Join(dir, imagesDir), data)
}

// imageSessionDir is the session gate of the upload and delete routes: the
// session's state dir, or the refusal that stops the write.
func (p *Prompt) imageSessionDir(sessionID string) (string, wire.SessionImageResult, error) {
	dir, meta, res, err := p.sessions.SessionDir(sessionID)
	if res.Refused || err != nil {
		return "", imageRefusalOf(res), err
	}
	if !p.settings.Load().HasChatReader(meta.Harness) {
		return "", imageRefusal(wire.ReasonPromptHarnessNotSupported, fmt.Sprintf("session %s runs a harness with no chat", sessionID)), nil
	}
	// A write must work while the session is Working, which the prompt route
	// refuses with ReasonSessionNotIdle, so an Exited (or still starting)
	// session answers ReasonSessionNotLive instead.
	if st := meta.Info().Status.State; st != wire.SessionIdle && st != wire.SessionWorking {
		return "", imageRefusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s is %s", sessionID, st)), nil
	}
	return dir, wire.SessionImageResult{}, nil
}

func findImage(sessionDir, id string) (string, bool) {
	return imagestore.Find(filepath.Join(sessionDir, imagesDir), id)
}

// sentMarker is the empty file that marks an image as pasted into the session.
// A marker beside the image keeps the image's path, which the chat shows, and
// is not matched by imageFileName.
func sentMarker(sessionDir, id string) string {
	return filepath.Join(sessionDir, imagesDir, ".sent-"+id)
}

func imageSent(sessionDir, id string) bool {
	_, err := os.Stat(sentMarker(sessionDir, id))
	return err == nil
}

// resolveImages is the absolute paths of the saved, unsent images ids name, in
// order, or the refusal that stops the prompt. resend allows images already
// marked sent: a failed send's replay pastes them again.
func (p *Prompt) resolveImages(sessionDir string, ids []string, resend bool) ([]string, *wire.SessionResult) {
	if len(ids) > wire.MaxPromptImages {
		r := sessions.Refusal(wire.ReasonImageTooMany, fmt.Sprintf("at most %d images per prompt", wire.MaxPromptImages))
		return nil, &r
	}
	p.imageMu.Lock()
	defer p.imageMu.Unlock()
	paths := make([]string, 0, len(ids))
	for _, id := range ids {
		path, ok := findImage(sessionDir, id)
		if !ok {
			r := sessions.Refusal(wire.ReasonImageNotFound, fmt.Sprintf("no image %s in this session", id))
			return nil, &r
		}
		if (!resend && imageSent(sessionDir, id)) || slices.Contains(ids[:len(paths)], id) {
			r := sessions.Refusal(wire.ReasonImageSent, fmt.Sprintf("image %s was already sent", id))
			return nil, &r
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// markImagesSent records that ids were pasted.
func (p *Prompt) markImagesSent(sessionDir string, ids []string) error {
	p.imageMu.Lock()
	defer p.imageMu.Unlock()
	for _, id := range ids {
		if err := os.WriteFile(sentMarker(sessionDir, id), nil, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// imageServeHandler serves GET /v1/sessions/images?session=<id>&id=<image>.
// Any miss, in the session or the image, is a 404.
func (p *Prompt) imageServeHandler(w http.ResponseWriter, r *http.Request) {
	dir, _, res, err := p.sessions.SessionDir(r.URL.Query().Get("session"))
	if res.Refused || err != nil {
		http.NotFound(w, r)
		return
	}
	path, ok := findImage(dir, r.URL.Query().Get("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", imagestore.DetectType(data))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	_, _ = w.Write(data)
}

// imageDeleteHandler serves POST /v1/sessions/images/delete.
func (p *Prompt) imageDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req wire.SessionImageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := p.deleteSessionImage(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

// deleteSessionImage removes a saved image under the upload's session gates.
// A refusal of an already-sent image goes here, after the lookup.
func (p *Prompt) deleteSessionImage(req wire.SessionImageRequest) (wire.SessionImageResult, error) {
	dir, gate, err := p.imageSessionDir(req.ID)
	if gate.Refused || err != nil {
		return gate, err
	}
	p.imageMu.Lock()
	defer p.imageMu.Unlock()
	if _, ok := findImage(dir, req.Image); !ok {
		return imageRefusal(wire.ReasonImageNotFound, fmt.Sprintf("no image %s in session %s", req.Image, req.ID)), nil
	}
	if imageSent(dir, req.Image) {
		return imageRefusal(wire.ReasonImageSent, fmt.Sprintf("image %s was already sent", req.Image)), nil
	}
	name, _, err := imagestore.Remove(filepath.Join(dir, imagesDir), req.Image)
	if err != nil {
		return wire.SessionImageResult{}, err
	}
	return wire.SessionImageResult{ID: req.Image, Name: name}, nil
}

var imageRefusal = imagestore.Refusal

func imageRefusalOf(res wire.SessionResult) wire.SessionImageResult {
	return wire.SessionImageResult{Refused: res.Refused, Reason: res.Reason, Message: res.Message}
}
