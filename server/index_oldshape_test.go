package server

import (
	"os"
	"path/filepath"
	"testing"
)

// oldShapeStore is a store whose one ticket the strict scan rejects.
func oldShapeStore(t *testing.T) string {
	t.Helper()
	store := t.TempDir()
	dir := filepath.Join(store, "proj")
	if err := os.MkdirAll(filepath.Join(dir, "epic", "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	ticket := "---\nid: \"01\"\nstatus: open\ntype: implement\n---\nBody.\n"
	if err := os.WriteFile(filepath.Join(dir, "epic", "issues", "01-old.md"), []byte(ticket), 0o644); err != nil {
		t.Fatal(err)
	}
	// The loader is tolerant of ticket content today, so a stray file where
	// the archive directory belongs stands in for what the strict scan rejects.
	if err := os.WriteFile(filepath.Join(dir, ".archive"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return store
}

// Seam: the first index of an old-shape store is empty, not an error; the
// scan that runs after the migrations fills it.
func TestBuildIndex_OldShapeTicketGivesEmptyIndex(t *testing.T) {
	t.Parallel()
	store := oldShapeStore(t)
	if _, _, err := scanStore(store); err == nil {
		t.Fatal("strict scan accepted the old-shape ticket; fix the fixture")
	}
	idx, err := buildIndex(store, newBroker(1))
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if len(idx.list) != 0 {
		t.Errorf("list = %v, want empty", idx.list)
	}
}
