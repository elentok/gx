package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateEpic_RejectsCallerCreatedAtAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	fields := EpicFields{Kind: KindLoop, CreatedAt: time.Now()}

	_, err := CreateEpic(t.TempDir(), root, "x", fields, "")
	if err == nil || !strings.Contains(err.Error(), "server-stamped") {
		t.Fatalf("err = %v, want server-stamped refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "x")); statErr == nil {
		t.Error("refusal wrote an epic directory")
	}
}
