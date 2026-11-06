package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/broker"
)

// Seam: the sum the index holds describes the file's bytes, so the claim check
// can compare it with FileSum.
func TestSumOf_EqualsFileSumAfterScan(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	dir := filepath.Join(store, "proj")
	if err := os.MkdirAll(filepath.Join(dir, "epic", "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "epic", "issues", "01-first.md")
	ticket := "---\nid: \"01\"\nstatus: open\ntype: implement\n---\nBody.\n"
	if err := os.WriteFile(path, []byte(ticket), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := New(store, broker.New(1), nil)
	if err := idx.Refresh(); err != nil {
		t.Fatal(err)
	}
	want, err := FileSum(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := idx.SumOf("proj:epic/01")
	if !ok || got != want {
		t.Fatalf("SumOf = %q, %v; want %q", got, ok, want)
	}
}
