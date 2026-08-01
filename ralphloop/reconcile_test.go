package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// testReconcileParams builds a reconcileParams for tests that don't exercise
// the doneRecoverable repair path, wiring just enough (a fresh FeatureLock,
// no gate/resume-signal needed since repair never pauses in these fixtures)
// for reconcile to run.
func testReconcileParams(workspaceID string, paths reconcilePaths, report func(string, ...any)) reconcileParams {
	return reconcileParams{
		WorkspaceID: workspaceID,
		Paths:       paths,
		Agent:       AgentClaude,
		SmartZone:   defaultSmartZone,
		Gate:        newPauseGate(),
		FeatureLock: &sync.Mutex{},
		Report:      report,
	}
}

func TestReconcile_ClaimedWithNoLiveTab_RevertsToOpen(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** claimed\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return nil, nil // no live tabs at all
	}

	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, func(string, ...any) {}), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Fatalf("reattached = %v, want none", reattached)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "Status:** open") {
		t.Errorf("ticket not reverted to open:\n%s", raw)
	}
}

func TestReconcile_ClaimedWithLiveTab_ReturnsReattached(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** claimed\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}

	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, func(string, ...any) {}), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 1 || reattached[0].Number != 1 {
		t.Fatalf("reattached = %v, want ticket 01", reattached)
	}

	// Status: claimed must be left untouched — the caller resumes it, it's not
	// reverted to open.
	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "Status:** claimed") {
		t.Errorf("ticket status changed unexpectedly:\n%s", raw)
	}
}

func TestReconcile_NeedsAttentionWithLiveTab_ReturnsReattached(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** needs-attention\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}
	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-iter-01", Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}

	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, func(string, ...any) {}), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 1 || reattached[0].Number != 1 {
		t.Fatalf("reattached = %v, want needs-attention ticket 01", reattached)
	}
}

func TestRun_NeedsAttentionWithoutLiveTab_DoesNotScheduleOtherTickets(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-attention.md": "# Attention\n\n**Status:** needs-attention\n",
		"02-open.md":      "# Open\n\n**Status:** open\n",
	})
	d, prompts, _ := fakeDeps()
	d.TabList = func(string) ([]herdr.Tab, error) { return nil, nil }

	err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("Run() error = %v, want actionable paused error", err)
	}
	if len(*prompts) != 0 {
		t.Errorf("prompts = %v, want no new iteration while ticket needs attention", *prompts)
	}
}

func TestRun_RestartedNeedsAttentionRecoversThenResumesScheduling(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-attention.md": "# Attention\n\n**Status:** needs-attention\n",
		"02-open.md":      "# Open\n\n**Status:** open\n",
	})
	d, prompts, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-iter-01", Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}
	var mu sync.Mutex
	sawClaimed := false
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		if strings.Contains(dir, "iter-01") {
			raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-attention.md"))
			mu.Lock()
			sawClaimed = err == nil && strings.Contains(string(raw), "claimed")
			mu.Unlock()
		}
		return 1, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &strings.Builder{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(*prompts) != 1 || !strings.HasSuffix((*prompts)[0], "02-open.md") {
		t.Errorf("prompts = %v, want only the newly scheduled second ticket", *prompts)
	}
	mu.Lock()
	defer mu.Unlock()
	if !sawClaimed {
		t.Error("recovered ticket was not restored to claimed before completion")
	}
}

func TestReconcile_OpenAndDoneTicketsIgnored(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
		"02-b.md": "# B\n\n**Status:** done\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return nil, nil
	}

	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, func(string, ...any) {}), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Fatalf("reattached = %v, want none", reattached)
	}
}

// classifyDoneTicketFixture is the shared setup for the four
// classifyDoneTicket outcomes: one done ticket, plus a Deps whose
// RevParse/IsAncestor/WorktreeExists are overridden per-case below.
func classifyDoneTicketFixture() (Deps, tickets.Ticket) {
	d, _, _ := fakeDeps()
	return d, tickets.Ticket{Number: 3, Status: "done"}
}

func TestClassifyDoneTicket_CommitLandedNoLeftover_OK(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return true, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	events := []Event{{Type: eventCherryPicked, Ticket: 3, SHA: "abc123"}}

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, events, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneOK {
		t.Errorf("class = %v, want doneOK", class)
	}
}

func TestClassifyDoneTicket_CommitLandedButBranchLeftover_StaleCleanup(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return true, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "deadbeef", nil } // iteration branch still exists
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	events := []Event{{Type: eventCherryPicked, Ticket: 3, SHA: "abc123"}}

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, events, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneStaleCleanup {
		t.Errorf("class = %v, want doneStaleCleanup", class)
	}
}

func TestClassifyDoneTicket_CommitMissingBranchStillHasIt_Recoverable(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "deadbeef", nil }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	events := []Event{{Type: eventCherryPicked, Ticket: 3, SHA: "abc123"}}

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, events, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneRecoverable {
		t.Errorf("class = %v, want doneRecoverable", class)
	}
}

func TestClassifyDoneTicket_CommitMissingNoBranch_Unrecoverable(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	events := []Event{{Type: eventCherryPicked, Ticket: 3, SHA: "abc123"}}

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, events, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneUnrecoverable {
		t.Errorf("class = %v, want doneUnrecoverable", class)
	}
}

func TestClassifyDoneTicket_NoRecordedEvent_TreatedAsMissing(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) {
		t.Fatal("IsAncestor should not be called with no recorded SHA to check")
		return false, nil
	}
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, nil, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneUnrecoverable {
		t.Errorf("class = %v, want doneUnrecoverable when no event was ever logged", class)
	}
}

func TestClassifyDoneTicket_LiveTabCountsAsLeftover(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return true, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	events := []Event{{Type: eventCherryPicked, Ticket: 3, SHA: "abc123"}}

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, events, map[string]bool{iterLabel(3): true})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneStaleCleanup {
		t.Errorf("class = %v, want doneStaleCleanup when the iteration tab is still live", class)
	}
}

// TestReconcile_DoneTicketMismatch_ReportedNotRepaired exercises the full
// reconcile() entrypoint (not just classifyDoneTicket in isolation): a done
// ticket whose landed commit went missing is reported, but reconcile doesn't
// touch its status or the reattached list — repair is a later ticket's job.
func TestReconcile_DoneTicketMismatch_ReportedNotRepaired(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "# C\n\n**Status:** done\n",
	})
	if err := logEvent(scratchDir, "epic", Event{Type: eventCherryPicked, Ticket: 3, SHA: "abc123"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }

	var reports []string
	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, func(format string, args ...any) {
		reports = append(reports, fmt.Sprintf(format, args...))
	}), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Errorf("reattached = %v, want none for a done ticket", reattached)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "03-c.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "Status:** done") {
		t.Errorf("ticket status changed unexpectedly:\n%s", raw)
	}

	found := false
	for _, r := range reports {
		if strings.Contains(r, "ticket 03") && strings.Contains(r, "no iteration branch left") {
			found = true
		}
	}
	if !found {
		t.Errorf("reports = %v, want an unrecoverable-mismatch report for ticket 03", reports)
	}
}

// realGitDeps wires IsAncestor/RevParse/WorktreeExists to the real git
// package and filesystem, leaving every other Deps field faked — used by the
// classifyDoneTicket tests below that need genuine git plumbing (patch
// hashes, ancestor checks) rather than a closure asserting the answer it was
// told to give.
func realGitDeps() Deps {
	d, _, _ := fakeDeps()
	d.IsAncestor = git.IsAncestor
	d.RevParse = git.RevParse
	d.WorktreeExists = worktreeExists
	return d
}

// TestClassifyDoneTicket_RealRepo_CommitLandedAndCleanedUp_OK exercises
// classifyDoneTicket against an actual git repo: an iteration branch's
// commit cherry-picked onto the feature branch (a fresh commit hash, exactly
// as CherryPickRange produces in production), then the iteration branch
// itself deleted — the fully-cleaned-up state a future cleanup step leaves
// behind. Verifies the real git.IsAncestor/git.RevParse wiring, not just the
// classifier's own branching logic.
func TestClassifyDoneTicket_RealRepo_CommitLandedAndCleanedUp_OK(t *testing.T) {
	dir := testutil.TempRepo(t)
	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "ralph-loop/iter-03", base)
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")
	iterTip, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "main")
	if err := git.CherryPickRange(dir, base, iterTip); err != nil {
		t.Fatalf("CherryPickRange: %v", err)
	}
	landedSHA, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "branch", "-D", "ralph-loop/iter-03")

	d := realGitDeps()
	events := []Event{{Type: eventCherryPicked, Ticket: 3, SHA: landedSHA}}
	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: dir, WorktreeDir: "/fake/worktrees"}, "main", tickets.Ticket{Number: 3, Status: "done"}, events, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneOK {
		t.Errorf("class = %v, want doneOK", class)
	}
}

// TestClassifyDoneTicket_RealRepo_NeverCherryPicked_Recoverable exercises
// classifyDoneTicket against a real repo where the iteration branch's commit
// was never cherry-picked onto the feature branch at all (no run-log event,
// matching a crash before that step), but the iteration branch itself is
// still around to recover it from.
func TestClassifyDoneTicket_RealRepo_NeverCherryPicked_Recoverable(t *testing.T) {
	dir := testutil.TempRepo(t)
	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "ralph-loop/iter-03", base)
	testutil.WriteFile(t, dir, "a.txt", "a\n")
	testutil.CommitAll(t, dir, "add a")

	testutil.MustGitExported(t, dir, "checkout", "main")

	d := realGitDeps()
	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: dir, WorktreeDir: "/fake/worktrees"}, "main", tickets.Ticket{Number: 3, Status: "done"}, nil, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneRecoverable {
		t.Errorf("class = %v, want doneRecoverable", class)
	}
}

// TestRun_RestartWithClaimedTicketButNoLiveTab_RevertsAndRerunsFromScratch
// exercises the full Run() path (not just reconcile in isolation): a ticket
// left claimed by a prior crashed invocation, with nothing live in the herdr
// workspace, is reverted to open and then picked up and run fresh by normal
// scheduling.
func TestRun_RestartWithClaimedTicketButNoLiveTab_RerunsFromScratch(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** claimed\n",
	})
	d, prompts, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return nil, nil
	}

	var out strings.Builder
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*prompts) != 1 || !strings.HasSuffix((*prompts)[0], "01-a.md") {
		t.Fatalf("prompts = %v, want ticket 01 run fresh", *prompts)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "Status:** done") {
		t.Errorf("ticket not landed after restart:\n%s", raw)
	}
}

// TestRun_RestartWithClaimedTicketAndLiveTab_ReattachesWithoutReplayingPrompt
// verifies the other half of ticket 08: a claimed ticket whose iter-NN tab is
// still alive is reattached (no fresh worktree create, no initial prompt
// replayed) and driven through to completion.
func TestRun_RestartWithClaimedTicketAndLiveTab_ReattachesWithoutReplayingPrompt(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** claimed\n",
	})
	d, prompts, removed := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-iter-01", Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}

	var worktreeCreateCalledForIter bool
	origAddWorktree := d.AddWorktree
	d.AddWorktree = func(repoDir, path, branch, base string) error {
		if strings.Contains(path, "iter-01") {
			worktreeCreateCalledForIter = true
		}
		return origAddWorktree(repoDir, path, branch, base)
	}

	var out strings.Builder
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if worktreeCreateCalledForIter {
		t.Error("a reattached iteration must reuse its existing worktree, not create a new one")
	}
	if len(*prompts) != 0 {
		t.Errorf("prompts = %v, want no initial prompt replayed for a reattached iteration", *prompts)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the reattached iteration's worktree removed on completion", *removed)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "Status:** done") {
		t.Errorf("reattached ticket not marked done:\n%s", raw)
	}
}

// TestReconcile_DoneTicketRecoverable_AutoRecherryPicksAndReports exercises
// ticket 02: a done ticket classified doneRecoverable (its landed commit
// missing from the feature branch, but its iteration branch still holds it)
// gets re-cherry-picked automatically, with a cherry-picked event logged and
// a report line naming what was restored.
func TestReconcile_DoneTicketRecoverable_AutoRecherryPicksAndReports(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "# C\n\n**Status:** done\n",
	})
	if err := logEvent(scratchDir, "epic", Event{Type: eventCherryPicked, Ticket: 3, SHA: "abc123"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil } // landed SHA missing
	// d.RevParse defaults to returning "deadbeef" for any ref (fakeDeps), so
	// the iteration branch is treated as still existing.

	var picked []string
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		picked = append(picked, fromExclusive+".."+toInclusive)
		return nil
	}

	var reports []string
	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, func(format string, args ...any) {
		reports = append(reports, fmt.Sprintf(format, args...))
	}), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Errorf("reattached = %v, want none for a done ticket", reattached)
	}
	if len(picked) != 1 {
		t.Fatalf("CherryPickRange calls = %v, want exactly one re-cherry-pick", picked)
	}

	found := false
	for _, r := range reports {
		if strings.Contains(r, "ticket 03") && strings.Contains(r, "restored") {
			found = true
		}
	}
	if !found {
		t.Errorf("reports = %v, want a report line naming ticket 03 as restored", reports)
	}

	events, _, err := readEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	sawRepairCherryPick := false
	for _, ev := range events {
		if ev.Type == eventCherryPicked && ev.Ticket == 3 && ev.SHA == "deadbeef" {
			sawRepairCherryPick = true
		}
	}
	if !sawRepairCherryPick {
		t.Errorf("events = %v, want a cherry-picked event logged for the repair", events)
	}
}

// TestReconcile_DoneTicketRecoverable_ConflictGoesThroughResolutionPath
// verifies the repair's re-cherry-pick reuses the exact same conflict-
// resolution path (launching a "/resolving-merge-conflicts" agent in the
// feature worktree) a normal iteration's first cherry-pick uses, rather than
// a separate repair-specific conflict handler.
func TestReconcile_DoneTicketRecoverable_ConflictGoesThroughResolutionPath(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "# C\n\n**Status:** done\n",
	})
	if err := logEvent(scratchDir, "epic", Event{Type: eventCherryPicked, Ticket: 3, SHA: "abc123"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }

	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		return &fakeConflictErr{}
	}
	inProgress := true
	d.CherryPickInProgress = func(dir string) (bool, error) { return inProgress, nil }

	var resolutionPrompted bool
	origAgentPrompt := d.AgentPrompt
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if opts.Text == "/resolving-merge-conflicts" {
			resolutionPrompted = true
			inProgress = false
		}
		return origAgentPrompt(opts)
	}

	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, func(string, ...any) {}), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Errorf("reattached = %v, want none for a done ticket", reattached)
	}
	if !resolutionPrompted {
		t.Error("expected a /resolving-merge-conflicts agent to be prompted on cherry-pick conflict during repair")
	}
}

// TestReconcile_DoneTicketRecoverable_CleansUpLeftoverWorktreeAndTab verifies
// that once a doneRecoverable ticket's commits are repaired, its leftover
// iteration worktree/tab (if the crash left any behind) are removed/closed —
// branch deletion is left to a later ticket.
func TestReconcile_DoneTicketRecoverable_CleansUpLeftoverWorktreeAndTab(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "# C\n\n**Status:** done\n",
	})
	if err := logEvent(scratchDir, "epic", Event{Type: eventCherryPicked, Ticket: 3, SHA: "abc123"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-iter-03", Label: "iter-03", WorkspaceID: workspaceID}}, nil
	}
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.WorktreeExists = func(path string) (bool, error) { return strings.Contains(path, "iter-03"), nil }

	var removedWorktree string
	d.RemoveWorktree = func(repoDir, path string, force bool) error {
		removedWorktree = path
		return nil
	}
	var closedTab string
	d.TabClose = func(tabID string) error {
		closedTab = tabID
		return nil
	}

	_, err = reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees", RepoDir: "/fake/repo"}, func(string, ...any) {}), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	if !strings.Contains(removedWorktree, "iter-03") {
		t.Errorf("removedWorktree = %q, want the leftover iter-03 worktree removed", removedWorktree)
	}
	if closedTab != "tab-iter-03" {
		t.Errorf("closedTab = %q, want the leftover iter-03 tab closed", closedTab)
	}
}
