package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingFile_KeepsAtMostMaxFiles(t *testing.T) {
	t.Parallel()
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

// Seam: Config.LogLevel decides whether s.log.Debug lines reach the server log.
func TestServer_LogLevelGatesDebugLines(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		level slog.Level
		want  bool
	}{
		{"default is info", slog.LevelInfo, false},
		{"debug", slog.LevelDebug, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, err := os.MkdirTemp("", "gxs")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(base) })
			stateDir := filepath.Join(base, "s")
			srv, err := New(Config{StateDir: stateDir, Build: "b", LogLevel: tc.level})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- srv.Serve(ctx) }()
			srv.log.Debug("debug-probe")
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(LogPath(stateDir))
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(string(data), "debug-probe"); got != tc.want {
				t.Errorf("debug line present = %v, want %v\n%s", got, tc.want, data)
			}
		})
	}
}

func TestServer_WritesJSONLogToStateDir(t *testing.T) {
	t.Parallel()
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
