package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPruneScratchWorkspace_DeletesOnlyArchivedTicketsSubdirs(t *testing.T) {
	store := filepath.Join(t.TempDir(), "tickets")
	workspace := ScratchWorkspace(store)
	for _, d := range []string{filepath.Join(store, ScratchProject, "live"), filepath.Join(workspace, "live"), filepath.Join(workspace, "archived")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneScratchWorkspace(store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "live")); err != nil {
		t.Errorf("live ticket's subdir removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "archived")); !os.IsNotExist(err) {
		t.Errorf("archived ticket's subdir kept: %v", err)
	}
}
