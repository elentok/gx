package tickets

import (
	"os"
	"path/filepath"
	"testing"
)

const movedTicket = "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n\n# 01 — One\n"

func TestApplyMove_CarriesLogAndToleratesNone(t *testing.T) {
	for _, withLog := range []bool{true, false} {
		root := t.TempDir()
		oldPath := filepath.Join(root, "src", "issues", "01-one.md")
		writeFile(t, oldPath, movedTicket)
		const log = "{\"a\":1}\n{\"b\":2}\n"
		if withLog {
			writeFile(t, LogPath(oldPath), log)
		}
		src := Ticket{Identifier: "01", Path: oldPath}
		writeFile(t, filepath.Join(root, "dst", "issues", ".keep"), "")

		a := Address{Project: "p", Epic: "src", ID: "01"}
		p, err := PlanMove(root, a, src, "dst", "", MoveOpts{})
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if err := ApplyMove(p); err != nil {
			t.Fatalf("apply (log=%v): %v", withLog, err)
		}
		got, err := os.ReadFile(LogPath(p.NewPath))
		if withLog && (err != nil || string(got) != log) {
			t.Errorf("moved log = %q, %v; want %q", got, err, log)
		}
		if !withLog && !os.IsNotExist(err) {
			t.Errorf("a log appeared from nowhere: %v", err)
		}
		if _, err := os.Stat(LogPath(oldPath)); !os.IsNotExist(err) {
			t.Errorf("old log still there: %v", err)
		}
	}
}

func TestPlanMove_ArchivedBlockerIsNotABreak(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "src", "issues", "01-one.md")
	writeFile(t, oldPath, "---\nid: \"01\"\nstatus: open\ntype: implement\nblocked_by:\n  - gone/01\n---\n\n# 01 — One\n")
	writeFile(t, filepath.Join(root, "dst", "issues", ".keep"), "")
	writeFile(t, filepath.Join(root, ".archive", "gone", "issues", ".keep"), "")

	a := Address{Project: "p", Epic: "src", ID: "01"}
	if _, err := PlanMove(root, a, Ticket{Identifier: "01", Path: oldPath}, "dst", "", MoveOpts{}); err != nil {
		t.Fatalf("plan: %v", err)
	}
}

func TestResumeMove_StillMovesLog(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "src", "issues", "01-one.md")
	newPath := filepath.Join(root, "dst", "issues", "01-one.md")
	const log = "{\"a\":1}\n"
	writeFile(t, oldPath, movedTicket)
	writeFile(t, LogPath(oldPath), log)
	writeFile(t, newPath, movedTicket) // the crash happened after this write

	from := Address{Project: "p", Epic: "src", ID: "01"}
	to := Address{Project: "p", Epic: "dst", ID: "01"}
	if err := ResumeMove(root, from, to, oldPath, newPath); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if got, err := os.ReadFile(LogPath(newPath)); err != nil || string(got) != log {
		t.Errorf("moved log = %q, %v; want %q", got, err, log)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old ticket still there: %v", err)
	}
}
