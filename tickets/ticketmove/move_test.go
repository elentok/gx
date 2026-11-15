package ticketmove

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/tickettest"
)

func planFrom(t *testing.T, root, epic, dst string, opts MoveOpts) (MovePlan, error) {
	t.Helper()
	epics, err := tickets.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range epics {
		if e.Name == epic {
			return PlanMove(root, tickets.Address{Project: "p", Epic: epic, ID: "01"}, e.Tickets[0], dst, "", opts)
		}
	}
	t.Fatalf("no epic %s", epic)
	return MovePlan{}, nil
}

func TestPlanMove_SourceKinds(t *testing.T) {
	const tk = "---\nid: \"01\"\nstatus: open\ntype: implement\noriginal_address: p:src/09\n---\n\n# 01\n"
	for _, tc := range []struct {
		name, meta string
		archive    bool
		refused    bool
	}{
		{"map without step", "kind: map\n", false, false},
		{"map charting", "kind: map\ndesign_step: chart\n", false, false},
		{"map past chart", "kind: map\ndesign_step: spec\n", false, true},
		{"map past chart archived", "kind: map\ndesign_step: spec\n", true, false},
		{"backlog", "kind: backlog\n", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tickettest.WriteFile(t, filepath.Join(root, "src", "ticket.md"), "---\nstatus: open\n"+tc.meta+"---\n\n# src\n")
			tickettest.WriteFile(t, filepath.Join(root, "src", "issues", "01-a.md"), tk)
			tickettest.WriteFile(t, filepath.Join(root, "dst", "ticket.md"), "---\nstatus: open\nkind: backlog\n---\n\n# dst\n")
			tickettest.WriteFile(t, filepath.Join(root, "other", "issues", "01-b.md"),
				"---\nid: \"01\"\nstatus: open\ntype: implement\nblocked_by:\n  - src/01\n---\n\n# b\n")

			plan, err := planFrom(t, root, "src", "dst", MoveOpts{Archive: tc.archive})
			if tc.refused {
				var ae *tickets.AddressError
				if !errors.As(err, &ae) || ae.Code != tickets.CodeMoveMapSource || !strings.Contains(err.Error(), "Design has started") {
					t.Fatalf("err = %v, want %s", err, tickets.CodeMoveMapSource)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := ApplyMove(plan); err != nil {
				t.Fatal(err)
			}
			if b, err := os.ReadFile(filepath.Join(root, "other", "issues", "01-b.md")); err != nil || !strings.Contains(string(b), "dst/01") {
				t.Errorf("blocked_by not rewritten: %s, %v", b, err)
			}
			if b, err := os.ReadFile(plan.NewPath); err != nil || !strings.Contains(string(b), "original_address: p:src/09") {
				t.Errorf("original_address lost: %s, %v", b, err)
			}
		})
	}
}

func TestPlanMove_IntoBacklog(t *testing.T) {
	root := t.TempDir()
	tickettest.WriteFile(t, filepath.Join(root, "src", "issues", "01-a.md"), "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n\n# 01\n")
	tickettest.WriteFile(t, filepath.Join(root, "backlog", "ticket.md"), "---\nstatus: open\nkind: backlog\n---\n\n# backlog\n")
	plan, err := planFrom(t, root, "src", "backlog", MoveOpts{})
	if err != nil || plan.To.Epic != "backlog" {
		t.Fatalf("plan = %+v, err = %v", plan, err)
	}
}

func TestPlanMove_ClaimedStillRefusedOutOfCharting(t *testing.T) {
	root := t.TempDir()
	tickettest.WriteFile(t, filepath.Join(root, "src", "ticket.md"), "---\nstatus: open\nkind: map\n---\n\n# src\n")
	tickettest.WriteFile(t, filepath.Join(root, "src", "issues", "01-a.md"), "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n\n# 01\n")
	tickettest.WriteFile(t, filepath.Join(root, "dst", "ticket.md"), "---\nstatus: open\n---\n\n# dst\n")
	_, err := planFrom(t, root, "src", "dst", MoveOpts{})
	var ae *tickets.AddressError
	if !errors.As(err, &ae) || ae.Code != tickets.CodeMoveStatus {
		t.Fatalf("err = %v, want %s", err, tickets.CodeMoveStatus)
	}
}

func TestRewriteMovedRef(t *testing.T) {
	for _, tc := range []struct{ ref, want string }{
		{"01", "src/01"},
		{"04a", "src/04a"},
		{"dst/02", "02"},
		{"mine:dst/02", "02"},
		{"other/02", "other/02"},
		{"ralph-loop/branch", "ralph-loop/branch"},
		{"main", "main"},
	} {
		if got := rewriteMovedRef(tc.ref, "src", "dst", "mine"); got != tc.want {
			t.Errorf("rewriteMovedRef(%q) = %q, want %q", tc.ref, got, tc.want)
		}
	}
}
