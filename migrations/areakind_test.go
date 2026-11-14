package migrations

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/config"
)

func TestDefault_AreaEpicKindStepIsLastAndChangesNoFile(t *testing.T) {
	names := Default.Names()
	if got := names[len(names)-1]; got != "area-epic-kind" {
		t.Fatalf("last step = %q, want area-epic-kind", got)
	}
	root := t.TempDir()
	epic := filepath.Join(root, "e", "ticket.md")
	if err := os.MkdirAll(filepath.Dir(epic), 0o755); err != nil {
		t.Fatal(err)
	}
	const body = "---\nkind: area\nstatus: open\ncreated_at: 2026-01-02T03:04:05Z\n---\n# E\n"
	if err := os.WriteFile(epic, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	applied := filepath.Join(t.TempDir(), AppliedFile)
	env := Env{Dirs: config.DirsFrom([]string{"HOME=" + t.TempDir()}), Root: root, LookupRoot: root, AppliedPath: applied, SkipApplied: true}

	if _, _, err := Default.Run(env); err != nil {
		t.Fatal(err)
	}
	first := readLines(t, applied)
	if n := len(slices.DeleteFunc(slices.Clone(first), func(l string) bool { return l != "area-epic-kind" })); n != 1 {
		t.Errorf("step recorded %d times in %v, want once", n, first)
	}
	if got, _ := os.ReadFile(epic); string(got) != body {
		t.Errorf("ticket.md changed:\n%s", got)
	}
	if _, _, err := Default.Run(env); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(readLines(t, applied), first) {
		t.Errorf("second run changed the list: %v", readLines(t, applied))
	}
}
