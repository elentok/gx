package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets/schema"
)

// reportIterationStatus writes iteration_status: value onto the ticket at
// path on every Runner wait of fakeDeps' d for label, simulating the agent's
// own `gx tickets set --iteration-status` call during the iteration. Other
// tickets in a multi-item run are unaffected.
func reportIterationStatus(t *testing.T, d Deps, path, label, value string) {
	t.Helper()
	onRunnerWait(d, func(s agentrunner.Session) {
		if s.Label != label {
			return
		}
		if err := updateTicket(path, func(tk *schema.Ticket) {
			tk.IterationStatus = schema.IterationStatus(value)
		}); err != nil {
			t.Errorf("seeding iteration_status on %s: %v", path, err)
		}
	})
}

// TestRun_AskingReport_PausesWithoutCherryPickEvenWithCommits pins ticket
// 07's ordering invariant: a paused (answer) report is honoured before commit
// counting, merging, or cleanup, so an agent that committed green work and
// then stopped to ask never has that work cherry-picked and marked done while
// the question is unanswered. It also pins ticket 13's pause cleanup: the
// worktree/tab are dropped (the pause may last a weekend, and there's no
// reason to hold them open across it), but the iteration branch itself
// survives — DeleteBranch must never be called — since a resume reattaches
// to it to merge both sides of the answer boundary in one pick.
func TestRun_AskingReport_PausesWithoutCherryPickEvenWithCommits(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, _, removed := fakeDeps()

	cherryPickCalled := false
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		cherryPickCalled = true
		return nil
	}
	branchDeleted := false
	d.DeleteBranch = func(repoDir, branch string) error {
		branchDeleted = true
		return nil
	}
	// The default fakeDeps CommitsAhead returns 1: commits are present, which
	// is exactly the case that must not be merged once paused (answer) has been
	// reported.
	reportIterationStatus(t, d, path, "epic-iter-01", "needs-answer")

	runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{})

	if cherryPickCalled {
		t.Errorf("CherryPickRange called, want no cherry-pick for an adopted needs-answer report")
	}
	wantRemoved := []string{"ralph-loop/epic-item-01"}
	if len(*removed) != 1 || (*removed)[0] != wantRemoved[0] {
		t.Errorf("removed worktree branches = %v, want %v (worktree/tab dropped across the pause)", *removed, wantRemoved)
	}
	if branchDeleted {
		t.Errorf("DeleteBranch called, want the iteration branch to survive the pause for a later resume")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	assertRawPaused(t, string(raw), schema.PauseClassAnswer, schema.PauseSelfReported)
	if strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket must not be marked done after an adopted needs-answer report:\n%s", raw)
	}
	if strings.Contains(string(raw), "commitless: true") {
		t.Errorf("needs-answer adoption must not set commitless:\n%s", raw)
	}
}

// TestRun_FinishedReport_ZeroCommits_DoesNotReachDone pins the other half of
// ADR 0019's invariant: an agent's finished report can start a merging but
// never conclude one on its own — the commit count still decides. A finished
// report with nothing merged takes the ordinary zero-commit path instead of
// gx trusting the report into done.
func TestRun_FinishedReport_ZeroCommits_DoesNotReachDone(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, _, _ := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	reportIterationStatus(t, d, path, "epic-iter-01", "finished")

	runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{})

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "status: done") {
		t.Errorf("a finished report with zero commits must not reach done through the report alone:\n%s", raw)
	}
	assertRawPaused(t, string(raw), schema.PauseClassAnswer, schema.PauseZeroCommit)
}
