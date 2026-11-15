package migrations

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
)

func TestDefault_VerifyStoriesStepIsLastAndBacklogEpicKindRewritesAreaKind(t *testing.T) {
	names := Default.Names()
	if got := names[len(names)-1]; got != "verify-stories" {
		t.Fatalf("last step = %q, want verify-stories", got)
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
	if n := len(slices.DeleteFunc(slices.Clone(first), func(l string) bool { return l != "backlog-epic-kind" })); n != 1 {
		t.Errorf("step recorded %d times in %v, want once", n, first)
	}
	got, _ := os.ReadFile(epic)
	if !strings.Contains(string(got), "kind: backlog") || strings.Contains(string(got), "kind: area") {
		t.Errorf("ticket.md kind not rewritten:\n%s", got)
	}
	if _, _, err := Default.Run(env); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(readLines(t, applied), first) {
		t.Errorf("second run changed the list: %v", readLines(t, applied))
	}
}
