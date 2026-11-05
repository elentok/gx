package prompt

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
)

const (
	// maxImageBytes is the size of the largest Prompt image the server keeps.
	maxImageBytes = 20 << 20
	// imageBodyCap is the route's own body cap, a little above maxImageBytes so
	// an image of exactly maxImageBytes fits and one byte more is read in full
	// and refused with a reason instead of cut off.
	imageBodyCap = maxImageBytes + 64<<10
	// imagesDir is the folder of a session's Prompt images, inside its state dir.
	imagesDir = "images"
)

// imageExts maps a type detected from the bytes to the saved file's extension.
var imageExts = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// imageUploadHandler serves POST /v1/sessions/images?id=<session>: the body is
// one image's raw bytes.
func (p *Prompt) imageUploadHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, imageBodyCap))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		wire.WriteJSON(w, imageRefusal(wire.ReasonImageTooLarge, fmt.Sprintf("image is over %d MB", maxImageBytes>>20)))
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

// saveSessionImage stores one Prompt image unchanged in the session's images
// folder as img-<n>-<id><ext>. The name is gx's: plain ASCII, never from the
// upload, with the extension from the type the bytes show.
func (p *Prompt) saveSessionImage(sessionID string, data []byte) (wire.SessionImageResult, error) {
	dir, refusal, err := p.imageSessionDir(sessionID)
	if refusal != nil || err != nil {
		return orZero(refusal), err
	}
	if len(data) > maxImageBytes {
		return imageRefusal(wire.ReasonImageTooLarge, fmt.Sprintf("image is over %d MB", maxImageBytes>>20)), nil
	}
	ext, ok := imageExts[detectType(data)]
	if !ok {
		return imageRefusal(wire.ReasonImageTypeNotAllowed, "only PNG, JPEG, GIF and WebP images are accepted"), nil
	}
	return p.writeImage(filepath.Join(dir, imagesDir), ext, data)
}

// imageSessionDir is the session gate of the upload and delete routes: the
// session's state dir, or the refusal that stops the write.
func (p *Prompt) imageSessionDir(sessionID string) (string, *wire.SessionImageResult, error) {
	dir, meta, res, err := p.sessions.SessionDir(sessionID)
	if res.Refused || err != nil {
		r := imageRefusalOf(res)
		return "", &r, err
	}
	if !p.settings.Load().HasChatReader(meta.Harness) {
		r := imageRefusal(wire.ReasonPromptHarnessNotSupported, fmt.Sprintf("session %s runs a harness with no chat", sessionID))
		return "", &r, nil
	}
	// A write must work while the session is Working, which the prompt route
	// refuses with ReasonSessionNotIdle, so an Exited (or still starting)
	// session answers ReasonSessionNotLive instead.
	if st := meta.Info().Status.State; st != wire.SessionIdle && st != wire.SessionWorking {
		r := imageRefusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s is %s", sessionID, st))
		return "", &r, nil
	}
	return dir, nil, nil
}

func orZero(r *wire.SessionImageResult) wire.SessionImageResult {
	if r == nil {
		return wire.SessionImageResult{}
	}
	return *r
}

// imageFileName matches a saved image's file name and captures its id.
var imageFileName = regexp.MustCompile(`^img-\d+-([0-9a-f]{8})\.[a-z]+$`)

// findImage is the path of the saved image id in the session's images folder.
// The id is matched against the names gx itself wrote, so no part of a request
// ever becomes a path: `..`, a slash or an unknown id finds nothing.
func findImage(sessionDir, id string) (string, bool) {
	entries, err := os.ReadDir(filepath.Join(sessionDir, imagesDir))
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if m := imageFileName.FindStringSubmatch(e.Name()); m != nil && m[1] == id && e.Type().IsRegular() {
			return filepath.Join(sessionDir, imagesDir, e.Name()), true
		}
	}
	return "", false
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
// order, or the refusal that stops the prompt.
func (p *Prompt) resolveImages(sessionDir string, ids []string) ([]string, *wire.SessionResult) {
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
		if imageSent(sessionDir, id) || slices.Contains(ids[:len(paths)], id) {
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
	h.Set("Content-Type", detectType(data))
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
	dir, refusal, err := p.imageSessionDir(req.ID)
	if refusal != nil || err != nil {
		return orZero(refusal), err
	}
	p.imageMu.Lock()
	defer p.imageMu.Unlock()
	path, ok := findImage(dir, req.Image)
	if !ok {
		return imageRefusal(wire.ReasonImageNotFound, fmt.Sprintf("no image %s in session %s", req.Image, req.ID)), nil
	}
	if imageSent(dir, req.Image) {
		return imageRefusal(wire.ReasonImageSent, fmt.Sprintf("image %s was already sent", req.Image)), nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return wire.SessionImageResult{}, err
	}
	return wire.SessionImageResult{ID: req.Image, Name: filepath.Base(path)}, nil
}

// detectType is the media type the bytes show, without any parameters.
func detectType(data []byte) string {
	mt, _, _ := strings.Cut(http.DetectContentType(data), ";")
	return mt
}

// writeImage creates the next img-<n>-<id><ext> in folder. imageMu makes the
// count and the create one step, so two uploads never share an <n>.
func (p *Prompt) writeImage(folder, ext string, data []byte) (wire.SessionImageResult, error) {
	p.imageMu.Lock()
	defer p.imageMu.Unlock()
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return wire.SessionImageResult{}, err
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		return wire.SessionImageResult{}, err
	}
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return wire.SessionImageResult{}, err
	}
	id := hex.EncodeToString(raw[:])
	name := fmt.Sprintf("img-%d-%s%s", len(entries)+1, id, ext)
	if err := os.WriteFile(filepath.Join(folder, name), data, 0o644); err != nil {
		return wire.SessionImageResult{}, err
	}
	return wire.SessionImageResult{ID: id, Name: name}, nil
}

func imageRefusal(reason, msg string) wire.SessionImageResult {
	return wire.SessionImageResult{Refused: true, Reason: reason, Message: msg}
}

func imageRefusalOf(res wire.SessionResult) wire.SessionImageResult {
	return wire.SessionImageResult{Refused: res.Refused, Reason: res.Reason, Message: res.Message}
}
