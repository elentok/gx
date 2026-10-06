package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingFile_KeepsAtMostMaxFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	r, err := openRotatingFile(path, 100, 3)
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

func TestServer_WritesJSONLogToStateDir(t *testing.T) {
	// Unix socket paths are capped near 100 bytes; t.TempDir() can exceed that.
	base, err := os.MkdirTemp("", "gxs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	stateDir := filepath.Join(base, "s")
	srv, err := New(Config{StateDir: stateDir, Build: "b"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(LogPath(stateDir))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"level":"INFO"`) || !strings.Contains(string(data), `"msg":"server started"`) {
		t.Errorf("unexpected log:\n%s", data)
	}
}
