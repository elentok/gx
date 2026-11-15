package migrations

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/config"
)

// A ticket written before ## Stories existed is valid as it is: a full run,
// including verify-stories, reports no change and leaves the file alone.
func TestDefault_TicketWithoutStoriesMigratesNothing(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "e", "issues", "01-t.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const body = "---\nid: \"01\"\nstatus: open\ntype: verify\n---\n# T\n\n## Verified\n\n- story 1 abc 2026-01-02T03:04:05Z\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	env := Env{Dirs: config.DirsFrom([]string{"HOME=" + t.TempDir()}), Root: root, LookupRoot: root, SkipApplied: true}

	changes, _, err := Default.Run(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("changes = %+v, want none", changes)
	}
	if got, _ := os.ReadFile(path); string(got) != body {
		t.Errorf("ticket changed:\n%s", got)
	}
}
