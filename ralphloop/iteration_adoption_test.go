package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets/schema"
)

// reportIterationStatus returns a Deps.AgentWait that first writes
// iteration_status: value onto the ticket at path (simulating the agent's own
// `gx tickets set --iteration-status` call during the iteration) and then
// falls through to the idle report every fakeDeps agent produces. It only
// fires for the target iteration's own pane so other tickets in a multi-item
// run are unaffected.
func reportIterationStatus(t *testing.T, path, target, value string) func(herdr.AgentWaitOptions) (herdr.Agent, error) {
	t.Helper()
	return func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		if opts.Target == "pane-"+target {
			if err := updateTicket(path, func(tk *schema.Ticket) {
				tk.IterationStatus = schema.IterationStatus(value)
			}); err != nil {
				t.Errorf("seeding iteration_status on %s: %v", path, err)
			}
		}
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}
}

// TestRun_NeedsAnswerReport_ParksWithoutCherryPickEvenWithCommits pins ticket
// 07's ordering invariant: a needs-answer report is honoured before commit
// counting, landing, or cleanup, so an agent that committed green work and
// then stopped to ask never has that work cherry-picked and marked done while
// the question is unanswered. It also pins ticket 13's park cleanup: the
// worktree/tab are dropped (the park may last a weekend, and there's no
// reason to hold them open across it), but the iteration branch itself
// survives — DeleteBranch must never be called — since a resume reattaches
// to it to land both sides of the answer boundary in one pick.
func TestRun_NeedsAnswerReport_ParksWithoutCherryPickEvenWithCommits(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
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
	// is exactly the case that must not be landed once needs-answer has been
	// reported.
	d.AgentWait = reportIterationStatus(t, path, "epic-iter-01", "needs-answer")

	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{})

	if cherryPickCalled {
		t.Errorf("CherryPickRange called, want no cherry-pick for an adopted needs-answer report")
	}
	wantRemoved := []string{"ralph-loop/epic-item-01"}
	if len(*removed) != 1 || (*removed)[0] != wantRemoved[0] {
		t.Errorf("removed worktree branches = %v, want %v (worktree/tab dropped across the park)", *removed, wantRemoved)
	}
	if branchDeleted {
		t.Errorf("DeleteBranch called, want the iteration branch to survive the park for a later resume")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket not marked needs-answer:\n%s", raw)
	}
	if strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket must not be marked done after an adopted needs-answer report:\n%s", raw)
	}
	if strings.Contains(string(raw), "commitless: true") {
		t.Errorf("needs-answer adoption must not set commitless:\n%s", raw)
	}
	if !strings.Contains(string(raw), "park_kind: self-reported") {
		t.Errorf("ticket not stamped park_kind: self-reported after adopted needs-answer report:\n%s", raw)
	}
}

// TestRun_FinishedReport_ZeroCommits_DoesNotReachDone pins the other half of
// ADR 0019's invariant: an agent's finished report can start a landing but
// never conclude one on its own — the commit count still decides. A finished
// report with nothing landed takes the ordinary zero-commit path instead of
// gx trusting the report into done.
func TestRun_FinishedReport_ZeroCommits_DoesNotReachDone(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")
	d, _, _ := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	d.AgentWait = reportIterationStatus(t, path, "epic-iter-01", "finished")

	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{})

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "status: done") {
		t.Errorf("a finished report with zero commits must not reach done through the report alone:\n%s", raw)
	}
	if !strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket not marked needs-answer after a zero-commit finished report:\n%s", raw)
	}
}
