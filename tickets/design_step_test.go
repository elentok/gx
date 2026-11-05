package tickets

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEpic(t *testing.T, root, name, fm, body string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "ticket.md")
	if err := os.WriteFile(path, []byte("---\n"+fm+"---\n\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDesignStep_RoundTripsThroughLoader(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeEpic(t, root, "good", "kind: map\ndesign_step: spec\n", "# good\n")
	writeEpic(t, root, "bad", "kind: map\ndesign_step: nope\n", "# bad\n")

	if got := LoadEpicTicketMD(filepath.Join(root, "good")); got.DesignStep != "spec" || len(got.FrontmatterErrs) != 0 {
		t.Errorf("good = %q, errs %v", got.DesignStep, got.FrontmatterErrs)
	}
	if got := LoadEpicTicketMD(filepath.Join(root, "bad")); len(got.FrontmatterErrs) != 1 {
		t.Errorf("bad errs = %v, want one", got.FrontmatterErrs)
	}
}
