package ticketops_test

import (
	"github.com/elentok/gx/server/servertest/rigs"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/servertest"
)

// The one-offs epic is a standing inbox: with every ticket done it is not
// stamped, merged or retro'd.
func TestCompleteRootIfDone_AllDoneOneOffsEpicNeverMerges(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", queue.OneOffsEpic, "01", "a", servertest.TicketOpts{Type: "prompt", Status: "done"})
	h := rigs.StartOneOffsHarness(t, store)
	start, _, _ := servertest.RegisterTurn(h)
	merged := false
	h.Server.Merger().SetMergeInto(func(git.Repo, []git.Worktree, string, string) (history.MergeOutcome, error) {
		merged = true
		return history.MergeOutcome{}, nil
	})
	epicMD := filepath.Join(store, "proj", queue.OneOffsEpic, "ticket.md")
	before, err := os.ReadFile(epicMD)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Server.Merger().CompleteRootOf("proj:one-offs/01"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(epicMD)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("one-offs epic ticket.md changed:\n%s", after)
	}
	if merged {
		t.Error("one-offs epic was merged")
	}
	if len(start()) != 0 {
		t.Errorf("an agent started (retro?): %v", start())
	}
}
