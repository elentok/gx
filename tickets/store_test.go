package tickets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/config"
)

func TestProjectDirs_UnknownLandingDoesNotBreakTheScan(t *testing.T) {
	store := t.TempDir()
	for name, body := range map[string]string{
		"a": `{"name": "a", "landing": "bogus"}`,
		"b": `{"name": "b"}`,
	} {
		dir := filepath.Join(store, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, config.ProjectFileName), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dirs, err := ProjectDirs(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 {
		t.Errorf("dirs = %v, want both projects", dirs)
	}
}
