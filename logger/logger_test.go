package logger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDebug_WritesUnderSetDir(t *testing.T) {
	state := t.TempDir()
	SetDir(state)
	t.Cleanup(func() { SetDir("") })

	Debug("hello %d", 1)

	data, err := os.ReadFile(filepath.Join(state, "gx.log"))
	if err != nil {
		t.Fatalf("debug log not in set dir: %v", err)
	}
	if len(data) == 0 {
		t.Error("debug log is empty")
	}
}

func TestDebug_WithoutDirWritesNothing(t *testing.T) {
	SetDir("")
	cwd := t.TempDir()
	t.Chdir(cwd)

	Debug("dropped")

	entries, err := os.ReadDir(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("Debug without a dir wrote %d entries", len(entries))
	}
}
