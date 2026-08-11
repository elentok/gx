package tickets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrate_IterationStatusRoundTripsUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my-epic", "issues", "01-first-ticket.md")
	original := "---\nid: \"01\"\nstatus: done\ntype: task\niteration_status: working\n---\nBody.\n"
	writeFile(t, path, original)

	result, err := Migrate(dir)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(result.Changes) != 0 {
		t.Fatalf("Changes = %v, want none", result.Changes)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(raw) != original {
		t.Errorf("file changed by migration: got %q, want %q", string(raw), original)
	}
}
