package merge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/config"
)

// Serial: swaps the package var readProjectFile.
func TestProjectRepoCache_ReadsProjectFileOnlyWhenItChanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, config.ProjectFileName)
	if err := os.WriteFile(path, []byte(`{"name":"proj","repo":"/r"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	reads := 0
	orig := readProjectFile
	readProjectFile = func(dir string) (config.ProjectFile, error) { reads++; return orig(dir) }
	t.Cleanup(func() { readProjectFile = orig })

	var c projectRepoCache
	for range 2 {
		if got := c.repoOf(dir); got == nil || *got != "/r" {
			t.Fatalf("repoOf = %v, want /r", got)
		}
	}
	if reads != 1 {
		t.Errorf("project.json read %d times over two snapshots, want 1", reads)
	}
}
