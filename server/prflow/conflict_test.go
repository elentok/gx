package prflow_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/testutil/ghfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func conflictingPR(t *testing.T, f *rigs.PRFlow, mergeable string) {
	t.Helper()
	openPR(f)
	f.GH.Update(func(s *ghfake.State) { s.PRs[0].Mergeable = mergeable })
	f.StampPR(t, 5, prURL)
}

func mergeTickets(f *rigs.PRFlow) []tickets.Ticket {
	var out []tickets.Ticket
	for _, tk := range tickets.LoadEpicAt(f.EpicDir()).Tickets {
		if tk.Type == string(schema.TypeMerge) {
			out = append(out, tk)
		}
	}
	return out
}

// finishWith marks the ticket done, with resolution as its Resolution section.
func finishWith(t *testing.T, f *rigs.PRFlow, tk tickets.Ticket, resolution string) {
	t.Helper()
	raw := strings.Replace(servertest.ReadFile(t, tk.Path), "status: "+tk.Status, "status: done", 1)
	if resolution != "" {
		raw += "\n## Resolution\n\n" + resolution + "\n"
	}
	if err := os.WriteFile(tk.Path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	f.Rescan(t)
}

func assertBlock(t *testing.T, f *rigs.PRFlow, kind schema.PauseReason) {
	t.Helper()
	e := tickets.LoadEpicTicketMD(f.EpicDir())
	if e.MergeBlockKind != string(kind) || e.MergeBlockReason == "" {
		t.Fatalf("block = %q %q, want %s with a reason", e.MergeBlockKind, e.MergeBlockReason, kind)
	}
}

func TestConflict_ConflictingPRForksOnePRModeMergeTicket(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	conflictingPR(t, f, "CONFLICTING")
	f.Tick(t)
	f.Tick(t)
	got := mergeTickets(f)
	if len(got) != 1 {
		t.Fatalf("merge tickets = %d, want exactly 1 while it is open", len(got))
	}
	body := servertest.ReadFile(t, got[0].Path)
	for _, want := range []string{"git fetch origin", "git rebase origin/epic-a", "git rebase origin/main", "--force-with-lease", "Outcome: foreign-conflict"} {
		if !strings.Contains(body, want) {
			t.Errorf("ticket lacks %q:\n%s", want, body)
		}
	}
}

func TestConflict_UnknownBehindAndCleanForkNothing(t *testing.T) {
	for _, mergeable := range []string{"UNKNOWN", "BEHIND", "MERGEABLE"} {
		f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
		conflictingPR(t, f, mergeable)
		f.Tick(t)
		if n := len(mergeTickets(f)); n != 0 {
			t.Errorf("%s: merge tickets = %d, want none", mergeable, n)
		}
	}
}

// threeFinishedMerges conflicts the PR and finishes three merge tickets, then
// ticks once more so the cap pauses the epic.
func threeFinishedMerges(t *testing.T, f *rigs.PRFlow) {
	t.Helper()
	conflictingPR(t, f, "CONFLICTING")
	for i := 1; i <= 3; i++ {
		f.Tick(t)
		got := mergeTickets(f)
		if len(got) != i {
			t.Fatalf("after conflict %d: merge tickets = %d, want %d", i, len(got), i)
		}
		finishWith(t, f, got[i-1], "")
	}
	f.Tick(t)
}

func TestConflict_ThreeMergeTicketsThenNeedsRebase(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	threeFinishedMerges(t, f)
	f.Tick(t)
	if n := len(mergeTickets(f)); n != 3 {
		t.Errorf("merge tickets = %d, want 3 at the cap", n)
	}
	assertBlock(t, f, schema.PauseNeedsRebase)
}

func TestConflict_ForeignConflictOutcomeStoresTheBlock(t *testing.T) {
	for _, typ := range []string{"merge", "ci-fix"} {
		t.Run(typ, func(t *testing.T) {
			f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
			servertest.WriteTicketWith(t, f.Store, "proj", "epic-a", "02", "push", servertest.TicketOpts{Type: typ, Status: "done"})
			path := filepath.Join(f.EpicDir(), "issues", "02-push.md")
			raw := servertest.ReadFile(t, path) + "\n## Resolution\n\nOutcome: foreign-conflict\n"
			if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
				t.Fatal(err)
			}
			conflictingPR(t, f, "MERGEABLE")
			f.Tick(t)
			assertBlock(t, f, schema.PausePRForeignConflict)
		})
	}
}

func TestConflict_MergeAgainOnABlockedPREpicForksAPRModeTicketPastTheCap(t *testing.T) {
	f := rigs.StartPRFlow(t, rigs.PRFlowOpts{})
	threeFinishedMerges(t, f)
	assertBlock(t, f, schema.PauseNeedsRebase)
	for _, tk := range tickets.LoadEpicAt(f.EpicDir()).Tickets {
		if tk.Status != string(schema.StatusDone) {
			finishWith(t, f, tk, "")
		}
	}
	if out, err := exec.Command("git", "-C", f.Origin.Local, "branch", "epic-a", "main").CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v\n%s", err, out)
	}
	res, err := f.H.Client.Tickets.EpicMerge(context.Background(), "proj:epic-a")
	if err != nil || res.Refused || res.MergeTicket == "" {
		t.Fatalf("merge again = %+v, %v; want a forked merge ticket", res, err)
	}
	got := mergeTickets(f)
	if len(got) != 4 {
		t.Fatalf("merge tickets = %d, want 4", len(got))
	}
	last := got[3]
	if body := servertest.ReadFile(t, last.Path); !strings.Contains(body, "--force-with-lease") || !last.Manual {
		t.Errorf("ticket is not a manual PR-mode merge ticket:\n%s", body)
	}
	if e := tickets.LoadEpicTicketMD(f.EpicDir()); !e.MergedAt.IsZero() || e.MergeBlockKind != "" {
		t.Errorf("epic merged_at=%v block=%q, want unmerged with the block cleared", e.MergedAt, e.MergeBlockKind)
	}
}
