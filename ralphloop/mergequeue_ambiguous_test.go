package ralphloop

import (
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// A feature branch carrying the ticket's trailer on a commit that is neither
// the recorded SHA nor patch-equivalent must pause, not count as merged.
func TestMergeOne_TrailerOnly_PausesAmbiguousMerge(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\n# A\n",
	})
	path := ticketPath(scratchDir, "epic", "01-a.md")
	repo := t.TempDir()
	gitIn(t, repo, "init", "-b", "epic")
	// testutil.CommitAll runs plain git, which has no identity on CI runners.
	gitIn(t, repo, "config", "user.email", "test@test.com")
	gitIn(t, repo, "config", "user.name", "test")
	gitIn(t, repo, "commit", "--allow-empty", "-m", "root")
	base := gitIn(t, repo, "rev-parse", "HEAD")
	gitIn(t, repo, "checkout", "-b", "iter")
	testutil.WriteFile(t, repo, "work.txt", "the real work\n")
	testutil.CommitAll(t, repo, "the real work")
	gitIn(t, repo, "checkout", "epic")
	testutil.WriteFile(t, repo, "other.txt", "something else\n")
	testutil.CommitAll(t, repo, "something else\n\n"+ticketTrailerKey+": "+ticketTrailerValue("epic", "01"))

	d := testDeps()
	d.Runner = runnerfake.NewRunner()
	lp := mergeQueueParams{
		RepoDir: repo, FeatureWorktree: repo, FeatureBranch: "epic", ScratchDir: scratchDir,
		WorktreeLock: &sync.Mutex{}, Gate: NewGate(), Sink: noopEventSink{},
	}
	r := mergeOne(d, lp, mergeJob{ticket: tickets.Ticket{Identifier: "01", Path: path}, base: base, branch: "iter", path: repo})
	if r.err != nil || r.mergeDeferred || r.pausedOnChild {
		t.Fatalf("mergeOne = %+v (err %v), want a clean paused outcome", r, r.err)
	}
	tk, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatal(err)
	}
	assertPaused(t, tk, schema.PauseClassRepair, schema.PauseAmbiguousMerge)
	if t.Failed() {
		t.FailNow()
	}
	if reason := latestPauseReason(scratchDir, "epic", "01"); !strings.Contains(reason, base) {
		t.Errorf("pause reason %q does not name base %s", reason, base)
	}
}

func TestPausedOutcome_NoKindNoReason_FallsBackHonestly(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: paused\ntype: implement\n---\n# A\n",
	})
	path := ticketPath(scratchDir, "epic", "01-a.md")
	tk, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatal(err)
	}
	one := OneIteration{Epic: "epic", ScratchDir: scratchDir, Ticket: tickets.Ticket{Identifier: "01", Path: path}}

	out := pausedOutcome(one, tk)
	if out.Kind != events.IterationError {
		t.Errorf("Kind = %q, want %q", out.Kind, events.IterationError)
	}
	if !strings.Contains(out.Reason, "no pause reason") || !strings.Contains(out.Reason, RunLogPath(scratchDir, "epic")) {
		t.Errorf("Reason = %q, want it to name the run log", out.Reason)
	}
}
