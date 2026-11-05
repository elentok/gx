package ticketmove

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/tickettest"
)

const movedTicket = "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n\n# 01 — One\n"

func TestApplyMove_CarriesLogAndToleratesNone(t *testing.T) {
	for _, withLog := range []bool{true, false} {
		root := t.TempDir()
		oldPath := filepath.Join(root, "src", "issues", "01-one.md")
		tickettest.WriteFile(t, oldPath, movedTicket)
		const log = "{\"a\":1}\n{\"b\":2}\n"
		if withLog {
			tickettest.WriteFile(t, tickets.LogPath(oldPath), log)
		}
		src := tickets.Ticket{Identifier: "01", Path: oldPath}
		tickettest.WriteFile(t, filepath.Join(root, "dst", "issues", ".keep"), "")

		a := tickets.Address{Project: "p", Epic: "src", ID: "01"}
		p, err := PlanMove(root, a, src, "dst", "", MoveOpts{})
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if err := ApplyMove(p); err != nil {
			t.Fatalf("apply (log=%v): %v", withLog, err)
		}
		got, err := os.ReadFile(tickets.LogPath(p.NewPath))
		if withLog && (err != nil || string(got) != log) {
			t.Errorf("moved log = %q, %v; want %q", got, err, log)
		}
		if !withLog && !os.IsNotExist(err) {
			t.Errorf("a log appeared from nowhere: %v", err)
		}
		if _, err := os.Stat(tickets.LogPath(oldPath)); !os.IsNotExist(err) {
			t.Errorf("old log still there: %v", err)
		}
	}
}

func TestApplyMove_WaitsForProjectMigration(t *testing.T) {
	tickettest.RequireWaitsForProjectMigration(t, func(t *testing.T, root string) (string, func() error) {
		project := filepath.Join(root, "proj")
		oldPath := filepath.Join(project, "src", "issues", "01-one.md")
		tickettest.WriteFile(t, filepath.Join(project, "project.json"), "{}")
		tickettest.WriteFile(t, oldPath, movedTicket)
		tickettest.WriteFile(t, filepath.Join(project, "dst", "issues", ".keep"), "")
		return project, func() error {
			p, err := PlanMove(project, tickets.Address{Project: "p", Epic: "src", ID: "01"}, tickets.Ticket{Identifier: "01", Path: oldPath}, "dst", "", MoveOpts{})
			if err != nil {
				return err
			}
			return ApplyMove(p)
		}
	})
}

func TestRewriteRefsInFile_WaitsForProjectMigration(t *testing.T) {
	tickettest.RequireWaitsForProjectMigration(t, func(t *testing.T, root string) (string, func() error) {
		project := filepath.Join(root, "proj")
		path := filepath.Join(project, "epic", "issues", "02-two.md")
		tickettest.WriteFile(t, filepath.Join(project, "project.json"), "{}")
		tickettest.WriteFile(t, path, "---\nid: \"02\"\nstatus: open\ntype: implement\nblocked_by:\n  - \"01\"\n---\n\n# 02\n")
		return project, func() error {
			changed, err := RewriteRefsInFile(path, func(ref string) string { return "epic/" + ref })
			if err == nil && !changed {
				err = os.ErrInvalid
			}
			return err
		}
	})
}

func TestPlanMove_ArchivedBlockerIsNotABreak(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "src", "issues", "01-one.md")
	tickettest.WriteFile(t, oldPath, "---\nid: \"01\"\nstatus: open\ntype: implement\nblocked_by:\n  - gone/01\n---\n\n# 01 — One\n")
	tickettest.WriteFile(t, filepath.Join(root, "dst", "issues", ".keep"), "")
	tickettest.WriteFile(t, filepath.Join(root, ".archive", "gone", "issues", ".keep"), "")

	a := tickets.Address{Project: "p", Epic: "src", ID: "01"}
	if _, err := PlanMove(root, a, tickets.Ticket{Identifier: "01", Path: oldPath}, "dst", "", MoveOpts{}); err != nil {
		t.Fatalf("plan: %v", err)
	}
}

func TestResumeMove_StillMovesLog(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "src", "issues", "01-one.md")
	newPath := filepath.Join(root, "dst", "issues", "01-one.md")
	const log = "{\"a\":1}\n"
	tickettest.WriteFile(t, oldPath, movedTicket)
	tickettest.WriteFile(t, tickets.LogPath(oldPath), log)
	tickettest.WriteFile(t, newPath, movedTicket) // the crash happened after this write

	from := tickets.Address{Project: "p", Epic: "src", ID: "01"}
	to := tickets.Address{Project: "p", Epic: "dst", ID: "01"}
	if err := ResumeMove(root, from, to, oldPath, newPath); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if got, err := os.ReadFile(tickets.LogPath(newPath)); err != nil || string(got) != log {
		t.Errorf("moved log = %q, %v; want %q", got, err, log)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old ticket still there: %v", err)
	}
}
