package tickets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackfillEpicTimes_StampsOnceAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	for name, fm := range map[string]string{
		"m":      "kind: map\nstatus: open",
		"m-impl": "kind: loop\nstatus: open",
		"solo":   "kind: loop\nstatus: open",
		"open-m": "kind: map\nstatus: open",
	} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ticket.md"), []byte("---\n"+fm+"\n---\n\n# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	changes, err := BackfillEpicTimes(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) == 0 {
		t.Fatal("no changes on first run")
	}
	for _, name := range []string{"m", "m-impl", "solo", "open-m"} {
		if LoadEpicTicketMD(filepath.Join(root, name)).CreatedAt.IsZero() {
			t.Errorf("%s: created_at not backfilled", name)
		}
	}
	m := LoadEpicTicketMD(filepath.Join(root, "m"))
	impl := LoadEpicTicketMD(filepath.Join(root, "m-impl"))
	if m.CompletedAt.IsZero() || !m.CompletedAt.Equal(impl.CreatedAt) {
		t.Errorf("map completed_at %v, impl created_at %v; want equal and set", m.CompletedAt, impl.CreatedAt)
	}
	if !LoadEpicTicketMD(filepath.Join(root, "open-m")).CompletedAt.IsZero() {
		t.Error("map without an impl epic got completed_at")
	}
	if !LoadEpicTicketMD(filepath.Join(root, "solo")).CompletedAt.IsZero() {
		t.Error("loop epic got completed_at")
	}

	changes, err = BackfillEpicTimes(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("rerun changed %v", changes)
	}
}
