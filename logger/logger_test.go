package logger

import (
	"os"
	"path/filepath"
	"testing"
)

// Seam C: the debug log lands under the XDG state dir.
func TestDebug_WritesUnderStateDir(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	Debug("hello %d", 1)

	data, err := os.ReadFile(filepath.Join(state, "gx", "gx.log"))
	if err != nil {
		t.Fatalf("debug log not in state dir: %v", err)
	}
	if len(data) == 0 {
		t.Error("debug log is empty")
	}
}
