package prompt

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
	dir, meta, res, err := p.sessions.SessionDir(sessionID)
	if res.Refused || err != nil {
		return imageRefusalOf(res), err
	}
	if !p.settings.Load().HasChatReader(meta.Harness) {
		return imageRefusal(wire.ReasonPromptHarnessNotSupported, fmt.Sprintf("session %s runs a harness with no chat", sessionID)), nil
	}
	// An upload must work while the session is Working, which the prompt route
	// refuses with ReasonSessionNotIdle, so an Exited (or still starting)
	// session answers ReasonSessionNotLive instead.
	if st := meta.Info().Status.State; st != wire.SessionIdle && st != wire.SessionWorking {
		return imageRefusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s is %s", sessionID, st)), nil
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
