package lifecycle_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/lifecycle"
)

func TestRotatingFile_KeepsAtMostMaxFiles(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "server.log")
	r, err := lifecycle.OpenRotatingFile(path, 100, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	line := strings.Repeat("x", 59) + "\n" // two lines overflow 100 bytes
	for range 20 {
		if _, err := r.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := filepath.Glob(path + "*")
	if len(files) != 3 {
		t.Fatalf("got %d files %v, want 3", len(files), files)
	}
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > 100 {
			t.Errorf("%s is %d bytes, over the cap", f, info.Size())
		}
	}
}
