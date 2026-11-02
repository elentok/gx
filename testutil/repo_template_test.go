package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallRepoTemplate_ConcurrentInstallsAllSucceed(t *testing.T) {
	for name, stale := range map[string]bool{"fresh": false, "stale dir without HEAD": true} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(evalDir(t, t.TempDir()), "template")
			if stale {
				if err := os.MkdirAll(filepath.Join(path, "leftover"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for range 8 {
				t.Run("install", func(t *testing.T) {
					t.Parallel()
					if got := installRepoTemplate(t, path); got != path {
						t.Errorf("path = %q, want %q", got, path)
					}
					if !templateReady(path) {
						t.Errorf("template at %s has no HEAD", path)
					}
				})
			}
		})
	}
}
