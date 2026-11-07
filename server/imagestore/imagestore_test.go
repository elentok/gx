package imagestore

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/wire"
)

var (
	pngBytes  = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 16)...)
	jpegBytes = append([]byte("\xff\xd8\xff\xe0"), make([]byte, 16)...)
	gifBytes  = append([]byte("GIF89a"), make([]byte, 16)...)
	webpBytes = append([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), make([]byte, 16)...)
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		ext    string
		reason string
	}{
		{"png", pngBytes, ".png", ""},
		{"jpeg", jpegBytes, ".jpg", ""},
		{"gif", gifBytes, ".gif", ""},
		{"webp", webpBytes, ".webp", ""},
		{"text", []byte("hello, not an image"), "", wire.ReasonImageTypeNotAllowed},
		{"too large", append(bytes.Clone(pngBytes), make([]byte, MaxBytes)...), "", wire.ReasonImageTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext, res := Check(tt.data)
			if ext != tt.ext || res.Reason != tt.reason || res.Refused != (tt.reason != "") {
				t.Fatalf("Check = %q, %+v; want ext %q reason %q", ext, res, tt.ext, tt.reason)
			}
		})
	}
}

func TestSaveFindRemove(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "images")
	first, err := Save(folder, pngBytes)
	if err != nil || first.Refused {
		t.Fatalf("first save = %+v, %v", first, err)
	}
	second, err := Save(folder, jpegBytes)
	if err != nil || second.Refused {
		t.Fatalf("second save = %+v, %v", second, err)
	}
	if want := "img-1-" + first.ID + ".png"; first.Name != want {
		t.Fatalf("first name = %q, want %q", first.Name, want)
	}
	if want := "img-2-" + second.ID + ".jpg"; second.Name != want {
		t.Fatalf("second name = %q, want %q", second.Name, want)
	}

	if _, res := Check(nil); !res.Refused {
		t.Fatal("empty body must be refused")
	}
	if res, _ := Save(folder, []byte("text")); !res.Refused || res.Reason != wire.ReasonImageTypeNotAllowed {
		t.Fatalf("text save = %+v", res)
	}

	path, ok := Find(folder, first.ID)
	if !ok || filepath.Base(path) != first.Name {
		t.Fatalf("Find = %q, %v", path, ok)
	}
	for _, id := range []string{"..", "../images", "a/b", "", "deadbeef"} {
		if p, ok := Find(folder, id); ok {
			t.Fatalf("Find(%q) = %q, want nothing", id, p)
		}
	}

	name, found, err := Remove(folder, first.ID)
	if err != nil || !found || name != first.Name {
		t.Fatalf("Remove = %q, %v, %v", name, found, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
	if _, found, _ := Remove(folder, first.ID); found {
		t.Fatal("second Remove found the image")
	}
}
