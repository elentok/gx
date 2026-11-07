// Package imagestore is the rules for Prompt images kept in a folder: which
// bytes are accepted, how a saved file is named, how an id is found and
// removed. Session images and draft images share it so the rules exist once.
package imagestore

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

// MaxBytes is the size of the largest Prompt image kept.
const MaxBytes = 20 << 20

// exts maps a type detected from the bytes to the saved file's extension.
var exts = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// folderLocks holds one mutex per folder, so a draft folder and a session
// folder never block each other.
var folderLocks sync.Map // folder path -> *sync.Mutex

func lockFor(folder string) *sync.Mutex {
	mu, _ := folderLocks.LoadOrStore(filepath.Clean(folder), &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// Refusal builds a refused result.
func Refusal(reason, msg string) wire.SessionImageResult {
	return wire.SessionImageResult{Refused: true, Reason: reason, Message: msg}
}

// TooLarge is the refusal for an image over MaxBytes.
func TooLarge() wire.SessionImageResult {
	return Refusal(wire.ReasonImageTooLarge, fmt.Sprintf("image is over %d MB", MaxBytes>>20))
}

// DetectType is the media type the bytes show, without any parameters.
func DetectType(data []byte) string {
	mt, _, _ := strings.Cut(http.DetectContentType(data), ";")
	return mt
}

// Check is the file extension data is saved with, or the refusal for a body
// that is too large or not a PNG, JPEG, GIF or WebP.
func Check(data []byte) (string, wire.SessionImageResult) {
	if len(data) > MaxBytes {
		return "", TooLarge()
	}
	ext, ok := exts[DetectType(data)]
	if !ok {
		return "", Refusal(wire.ReasonImageTypeNotAllowed, "only PNG, JPEG, GIF and WebP images are accepted")
	}
	return ext, wire.SessionImageResult{}
}

// Save checks data and stores it unchanged in folder as img-<n>-<id><ext>. The
// name is gx's: plain ASCII, never from the upload. The count and the create
// are one step per folder, so two saves never share an <n>.
func Save(folder string, data []byte) (wire.SessionImageResult, error) {
	ext, refusal := Check(data)
	if refusal.Refused {
		return refusal, nil
	}
	mu := lockFor(folder)
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return wire.SessionImageResult{}, err
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		return wire.SessionImageResult{}, err
	}
	// Only images count: a marker file beside them is not one.
	images := 0
	for _, e := range entries {
		if _, _, ok := transcript.ParseSavedImageName(e.Name()); ok {
			images++
		}
	}
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return wire.SessionImageResult{}, err
	}
	id := hex.EncodeToString(raw[:])
	name := transcript.SavedImageName(images+1, id, ext)
	if err := os.WriteFile(filepath.Join(folder, name), data, 0o644); err != nil {
		return wire.SessionImageResult{}, err
	}
	return wire.SessionImageResult{ID: id, Name: name}, nil
}

// Find is the path of the saved image id in folder. The id is matched against
// the names gx itself wrote, so no part of a request ever becomes a path: `..`,
// a slash or an unknown id finds nothing.
func Find(folder, id string) (string, bool) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if _, got, ok := transcript.ParseSavedImageName(e.Name()); ok && got == id && e.Type().IsRegular() {
			return filepath.Join(folder, e.Name()), true
		}
	}
	return "", false
}

// Remove deletes the saved image id from folder and reports its file name.
// found is false when folder holds no such image.
func Remove(folder, id string) (name string, found bool, err error) {
	path, ok := Find(folder, id)
	if !ok {
		return "", false, nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", true, err
	}
	return filepath.Base(path), true, nil
}
