package tickets

import (
	"os"
	"path/filepath"
	"strings"
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

func TestMigrateDesignStep(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	trunk := t.TempDir()
	if err := os.MkdirAll(filepath.Join(trunk, "docs", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trunk, "docs", "specs", "specced.md"), []byte("spec"), 0o644); err != nil {
		t.Fatal(err)
	}
	chart := writeEpic(t, root, "charting", "kind: map\nstatus: draft\ndestination_unconfirmed: true\n", "# charting\n")
	specced := writeEpic(t, root, "specced", "kind: map\nstatus: draft\n", "# specced\n")
	has := writeEpic(t, root, "has", "kind: map\ndesign_step: grill-findings\ndestination_unconfirmed: true\n", "# has\n")
	loop := "kind: loop\nstatus: open\n"
	loopPath := writeEpic(t, root, "looping", loop, "# looping\n")
	loopBefore, _ := os.ReadFile(loopPath)

	changes, err := MigrateDesignStep(root, DesignProbe{TrunkDir: trunk})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("changes = %v, want charting and specced", changes)
	}
	for path, want := range map[string]string{chart: "design_step: chart", specced: "design_step: review", has: "design_step: grill-findings"} {
		if raw, _ := os.ReadFile(path); !strings.Contains(string(raw), want) {
			t.Errorf("%s missing %q:\n%s", path, want, raw)
		}
	}
	if raw, _ := os.ReadFile(loopPath); string(raw) != string(loopBefore) {
		t.Errorf("loop epic rewritten:\n%s", raw)
	}

	again, err := MigrateDesignStep(root, DesignProbe{TrunkDir: trunk})
	if err != nil || len(again) != 0 {
		t.Errorf("rerun = %v, %v; want no changes", again, err)
	}
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
