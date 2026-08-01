package ralphloop

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
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
func testReconcileParams(workspaceID string, paths reconcilePaths, sink EventSink) reconcileParams {
	return reconcileParams{
		WorkspaceID: workspaceID,
		Paths:       paths,
		Agent:       AgentClaude,
		SmartZone:   defaultSmartZone,
		Gate:        NewGate(),
		FeatureLock: &sync.Mutex{},
		Sink:        sink,
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
	// No iteration branch was ever created for this claim (it never got as
	// far as running), so there's nothing to recover — only the plain revert
	// applies.
	d.RevParse = func(dir, ref string) (string, error) {
		if ref == iterBranch("01") {
			return "", fmt.Errorf("unknown revision")
		}
		return "deadbeef", nil
	}

	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, NewTextEventSink(io.Discard)), epics[0])
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

// TestReconcile_ClaimedWithNoLiveTabButUnlandedCommits_RecoversInstead covers
// the crash/restart scenario reconcileOrphanedClaim exists for: a claimed
// ticket's agent finished and committed to its iteration branch, but the
// invocation went away before the normal finishIteration cherry-pick ran (no
// live tab survives to reattach to). Reverting straight to open would leave
// that branch orphaned, only for a fresh attempt to collide with it later —
// so those unlanded commits must be landed here instead.
func TestReconcile_ClaimedWithNoLiveTabButUnlandedCommits_RecoversInstead(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** claimed\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, removed := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return nil, nil // no live tabs at all
	}
	// Default fakeDeps RevParse/CommitsAhead already report the iteration
	// branch as existing with commits ahead of base — simulating the
	// finished-but-uncherry-picked branch left behind.

	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, NewTextEventSink(io.Discard)), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Fatalf("reattached = %v, want none (recovered synchronously, not launched)", reattached)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "Status:** done") {
		t.Errorf("orphaned claim with unlanded commits not recovered to done:\n%s", raw)
	}

	if len(*removed) != 1 {
		t.Errorf("removed worktrees = %v, want exactly one removal (cleanup only after landing the recovered commits)", *removed)
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

	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, NewTextEventSink(io.Discard)), epics[0])
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

	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, NewTextEventSink(io.Discard)), epics[0])
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

	err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&strings.Builder{}))
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

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&strings.Builder{})); err != nil {
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

	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, NewTextEventSink(io.Discard)), epics[0])
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
	return d, tickets.Ticket{Number: 3, Identifier: "03", Status: "done"}
}

func TestClassifyDoneTicket_CommitLandedNoLeftover_OK(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return true, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}}

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
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}}

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
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}}

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
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}}

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

// TestIterLabelIterBranch_DistinctForLetteredSiblingsSharingNumber covers
// ticket 08: a mid-flight split keeps the same Number across every letter
// (e.g. 04 -> 04a/04b), so iterLabel/iterBranch must key off Identifier, not
// Number, or lettered siblings would collide on the same worktree/branch.
func TestIterLabelIterBranch_DistinctForLetteredSiblingsSharingNumber(t *testing.T) {
	a, b := tickets.Ticket{Number: 4, Identifier: "04a"}, tickets.Ticket{Number: 4, Identifier: "04b"}
	if iterLabel(a.Identifier) == iterLabel(b.Identifier) {
		t.Errorf("iterLabel(%q) == iterLabel(%q) = %q, want distinct labels for siblings sharing Number 4", a.Identifier, b.Identifier, iterLabel(a.Identifier))
	}
	if iterBranch(a.Identifier) == iterBranch(b.Identifier) {
		t.Errorf("iterBranch(%q) == iterBranch(%q) = %q, want distinct branches for siblings sharing Number 4", a.Identifier, b.Identifier, iterBranch(a.Identifier))
	}
}

// TestClassifyDoneTicket_LetteredSiblingsShareNumber_NotCrossAttributed
// covers the other half of ticket 08: classifyDoneTicket/latestCherryPickedSHA
// must attribute a run-log cherry-picked event to the sibling that logged it
// (by Identifier), not whichever sibling happens to share its Number. Two
// done tickets, 04a and 04b, both Number 4: 04a's commit landed on the
// feature branch, 04b's didn't (and its iteration branch is gone). If SHA
// lookup were still keyed by Number, 04b would inherit 04a's landed SHA and
// misreport doneOK instead of doneUnrecoverable.
func TestClassifyDoneTicket_LetteredSiblingsShareNumber_NotCrossAttributed(t *testing.T) {
	d, _, _ := fakeDeps()
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) {
		return ancestor == "sha-04a", nil // only 04a's commit actually landed
	}
	d.RevParse = func(dir, ref string) (string, error) {
		return "", fmt.Errorf("unknown revision") // neither iteration branch survives
	}

	events := []Event{
		{Type: eventCherryPicked, Ticket: "04a", SHA: "sha-04a"},
		{Type: eventCherryPicked, Ticket: "04b", SHA: "sha-04b"},
	}
	paths := reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}

	classA, err := classifyDoneTicket(d, paths, "epic", tickets.Ticket{Number: 4, Identifier: "04a", Status: "done"}, events, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket(04a) error = %v", err)
	}
	if classA != doneOK {
		t.Errorf("classA = %v, want doneOK for 04a's own landed commit", classA)
	}

	classB, err := classifyDoneTicket(d, paths, "epic", tickets.Ticket{Number: 4, Identifier: "04b", Status: "done"}, events, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket(04b) error = %v", err)
	}
	if classB != doneUnrecoverable {
		t.Errorf("classB = %v, want doneUnrecoverable — 04b must not inherit 04a's landed SHA just because they share Number 4", classB)
	}
}

func TestClassifyDoneTicket_LiveTabCountsAsLeftover(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return true, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}}

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, events, map[string]bool{iterLabel("03"): true})
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
// TestReconcile_DoneTicketUnrecoverable_MarkedNeedsAttention exercises ticket
// 03: a done ticket classified doneUnrecoverable (its landed commit missing
// from the feature branch, no iteration branch left to recover it from) must
// not be silently reverted to open or left marked done — it's flagged
// needs-attention for a human to inspect, with a reason and a logged event.
func TestReconcile_DoneTicketUnrecoverable_MarkedNeedsAttention(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "# C\n\n**Status:** done\n",
	})
	if err := logEvent(scratchDir, "epic", Event{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}); err != nil {
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

	var out bytes.Buffer
	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, NewTextEventSink(&out)), epics[0])
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
	if !strings.Contains(string(raw), "Status:** needs-attention") {
		t.Errorf("ticket not marked needs-attention:\n%s", raw)
	}

	reports := strings.Split(out.String(), "\n")
	found := false
	for _, r := range reports {
		if strings.Contains(r, "ticket 03") && strings.Contains(r, "no iteration branch left") {
			found = true
		}
	}
	if !found {
		t.Errorf("reports = %v, want an unrecoverable-mismatch report for ticket 03", reports)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	if !ok {
		t.Fatalf("readEvents: run log not found")
	}
	var attentionEvent *Event
	for i := range events {
		if events[i].Type == eventNeedsAttention && events[i].Ticket == "03" {
			attentionEvent = &events[i]
		}
	}
	if attentionEvent == nil {
		t.Fatalf("events = %v, want a needs-attention event for ticket 3", events)
	}
	if attentionEvent.Reason == "" {
		t.Errorf("needs-attention event has no reason")
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
	d.MergeBase = git.MergeBase
	d.PatchesApplied = git.PatchesApplied
	d.AppendTrailer = git.AppendTrailer
	d.TrailerCommitExists = git.TrailerCommitExists
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
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: landedSHA}}
	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: dir, WorktreeDir: "/fake/worktrees"}, "main", tickets.Ticket{Number: 3, Identifier: "03", Status: "done"}, events, map[string]bool{})
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
	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: dir, WorktreeDir: "/fake/worktrees"}, "main", tickets.Ticket{Number: 3, Identifier: "03", Status: "done"}, nil, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneRecoverable {
		t.Errorf("class = %v, want doneRecoverable", class)
	}
}

// TestClassifyDoneTicket_RealRepo_RebasedAfterLanding_OK reproduces the
// postmortem scenario that motivated the PatchesApplied fallback: a ticket
// lands normally (cherry-picked, event recorded), but the feature branch is
// later rebased onto a newer upstream, rewriting the landed commit's hash out
// from under the recorded event's SHA — while the iteration branch (holding
// the pre-rebase original) never got cleaned up. Without the fallback,
// IsAncestor against the stale SHA reports false and the surviving iteration
// branch would make this misclassify as doneRecoverable, re-cherry-picking
// already-landed content onto the live feature worktree. PatchesApplied
// should recognize the rebased commit as patch-equivalent and classify this
// doneStaleCleanup instead — landed, just needs its leftover branch cleaned
// up.
func TestClassifyDoneTicket_RealRepo_RebasedAfterLanding_OK(t *testing.T) {
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

	// Simulate an unrelated upstream advance, then rebase main onto it — this
	// rewrites landedSHA to a new hash, exactly as happened to this repo's own
	// ralphloop-tui-impl branch.
	testutil.MustGitExported(t, dir, "checkout", "-b", "upstream", base)
	testutil.WriteFile(t, dir, "upstream.txt", "u\n")
	testutil.CommitAll(t, dir, "unrelated upstream change")
	testutil.MustGitExported(t, dir, "checkout", "main")
	testutil.MustGitExported(t, dir, "rebase", "upstream")

	// ralph-loop/iter-03 was never cleaned up, still pointing at the
	// pre-rebase original.
	d := realGitDeps()
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: landedSHA}}
	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: dir, WorktreeDir: "/fake/worktrees"}, "main", tickets.Ticket{Number: 3, Identifier: "03", Status: "done"}, events, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneStaleCleanup {
		t.Errorf("class = %v, want doneStaleCleanup — content already landed pre-rebase, just needs the leftover iteration branch cleaned up", class)
	}
}

// TestClassifyDoneTicket_RealRepo_RebasedWithConflictResolution_OK reproduces
// the scenario PatchesApplied still can't handle: the feature branch is
// rebased onto an upstream that conflicts with the already-landed commit, so
// landing it again requires a manually re-resolved merge — which changes the
// commit's hash *and* its diff (so PatchesApplied's patch-id comparison also
// reports "not found"), same as what happened to this repo's own tickets
// 01-03 after ralph-loop hit a real conflict re-landing a commit mid-rebase.
// The iteration branch is also gone by this point (already cleaned up), so
// PatchesApplied's hasBranch guard never even runs. Only the
// Ralph-Loop-Ticket trailer landCherryPick stamps on every landed commit
// survives a rebase's conflict-resolution message-preserving default,
// letting classifyDoneTicket still recognize this as landed instead of
// flagging a genuinely-done ticket doneUnrecoverable.
func TestClassifyDoneTicket_RealRepo_RebasedWithConflictResolution_OK(t *testing.T) {
	dir := testutil.TempRepo(t)
	base, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "-b", "ralph-loop/iter-03", base)
	testutil.WriteFile(t, dir, "a.txt", "iteration content\n")
	testutil.CommitAll(t, dir, "add a")
	iterTip, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	testutil.MustGitExported(t, dir, "checkout", "main")
	if err := git.CherryPickRange(dir, base, iterTip); err != nil {
		t.Fatalf("CherryPickRange: %v", err)
	}
	// landCherryPick always stamps the ticket trailer right after landing.
	if err := git.AppendTrailer(dir, ticketTrailerKey, "03"); err != nil {
		t.Fatalf("AppendTrailer: %v", err)
	}
	landedSHA, err := git.RevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}

	// An unrelated upstream commit that conflicts on the same file, then a
	// rebase forcing a manual re-resolution — this both rewrites landedSHA's
	// hash and changes its diff, defeating IsAncestor and PatchesApplied both.
	testutil.MustGitExported(t, dir, "checkout", "-b", "upstream", base)
	testutil.WriteFile(t, dir, "a.txt", "upstream content\n")
	testutil.CommitAll(t, dir, "unrelated conflicting change")
	testutil.MustGitExported(t, dir, "checkout", "main")
	// The rebase is expected to conflict — resolved by hand below, exactly
	// like cherryPickWithConflictResolution's real conflict-resolution path.
	rebaseCmd := exec.Command("git", "rebase", "upstream")
	rebaseCmd.Dir = dir
	_ = rebaseCmd.Run()
	testutil.WriteFile(t, dir, "a.txt", "resolved content\n")
	testutil.MustGitExported(t, dir, "add", "a.txt")
	continueCmd := exec.Command("git", "rebase", "--continue")
	continueCmd.Dir = dir
	continueCmd.Env = append(os.Environ(), "GIT_EDITOR=true")
	if out, err := continueCmd.CombinedOutput(); err != nil {
		t.Fatalf("git rebase --continue: %v\n%s", err, out)
	}

	// The iteration branch is already cleaned up by this point.
	testutil.MustGitExported(t, dir, "branch", "-D", "ralph-loop/iter-03")

	d := realGitDeps()
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: landedSHA}}
	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: dir, WorktreeDir: "/fake/worktrees"}, "main", tickets.Ticket{Number: 3, Identifier: "03", Status: "done"}, events, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneOK {
		t.Errorf("class = %v, want doneOK — trailer marker should recognize the rebased-and-reconflicted commit as landed", class)
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
	// No iteration branch was ever created for this claim, so reconcile has
	// nothing to recover — only the plain revert-then-rerun applies.
	d.RevParse = func(dir, ref string) (string, error) {
		if ref == iterBranch("01") {
			return "", fmt.Errorf("unknown revision")
		}
		return "deadbeef", nil
	}

	var out strings.Builder
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
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
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
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

// TestRun_RestartWithClaimedTicketAlreadyIdle_SkipsWaitAndCherryPicks
// verifies ticket 05: when a reattached iteration's pane already shows
// idle/done in TabList at reattach time (the agent finished while the
// previous invocation was down), the reattach proceeds straight to
// cherry-pick without ever polling AgentWait — a pane already sitting idle
// with no further status transition coming would otherwise wait out every
// poll timeout forever.
func TestRun_RestartWithClaimedTicketAlreadyIdle_SkipsWaitAndCherryPicks(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** claimed\n",
	})
	d, _, removed := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-iter-01", Label: "iter-01", WorkspaceID: workspaceID, AgentStatus: "idle"}}, nil
	}

	var agentWaitCalls int
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		agentWaitCalls++
		return herdr.Agent{AgentStatus: "idle"}, nil
	}

	var out strings.Builder
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if agentWaitCalls != 0 {
		t.Errorf("AgentWait calls = %d, want 0 for a pane already idle at reattach", agentWaitCalls)
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

	events, _, err := readEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	foundFinish := false
	for _, e := range events {
		if e.Type == eventIterationFinished && e.Ticket == "01" {
			foundFinish = true
		}
	}
	if !foundFinish {
		t.Errorf("events = %v, want an %s event logged for ticket 1 despite skipping the wait", events, eventIterationFinished)
	}
}

// TestRun_ReattachedClose_BackfillsContextAndSessionFromRunLog exercises
// ticket 06a: a reattached iteration has no live session id of its own this
// run, so its close backfills Context window/Session from the ticket's
// original iteration-started event in the run log instead of leaving them
// blank.
func TestRun_ReattachedClose_BackfillsContextAndSessionFromRunLog(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** claimed\n",
	})
	if err := logEvent(scratchDir, "epic", Event{
		Type:         eventIterationStarted,
		Ticket:       "01",
		Agent:        AgentClaude,
		AgentSession: "sess-original",
		Cwd:          "/fake/worktrees/iter-01",
	}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-iter-01", Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}
	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		if cwd == "/fake/worktrees/iter-01" && sessionID == "sess-original" {
			return 54321, true, nil
		}
		return 0, false, nil
	}

	var out strings.Builder
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "Status:** done") {
		t.Errorf("reattached ticket not marked done:\n%s", got)
	}
	if !strings.Contains(got, "Context window:** 54321") {
		t.Errorf("ticket missing backfilled context window field:\n%s", got)
	}
	if !strings.Contains(got, "Session:** sess-original") {
		t.Errorf("ticket missing backfilled session field:\n%s", got)
	}
}

// TestRun_ReattachedClose_NoPriorSessionInLog_OmitsMetadata verifies the
// ticket 06a edge case: when the run log has no iteration-started event to
// recover a session id from, the reattached close still marks the ticket
// done, without writing a blank or wrong Context window/Session field.
func TestRun_ReattachedClose_NoPriorSessionInLog_OmitsMetadata(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** claimed\n",
	})
	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-iter-01", Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}

	var out strings.Builder
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "Status:** done") {
		t.Errorf("reattached ticket not marked done:\n%s", got)
	}
	if strings.Contains(got, "Context window:") || strings.Contains(got, "Session:") {
		t.Errorf("ticket should omit metadata fields with no prior session in the run log, got:\n%s", got)
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
	if err := logEvent(scratchDir, "epic", Event{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}); err != nil {
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

	var out bytes.Buffer
	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, NewTextEventSink(&out)), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Errorf("reattached = %v, want none for a done ticket", reattached)
	}
	if len(picked) != 1 {
		t.Fatalf("CherryPickRange calls = %v, want exactly one re-cherry-pick", picked)
	}

	reports := strings.Split(out.String(), "\n")
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
		if ev.Type == eventCherryPicked && ev.Ticket == "03" && ev.SHA == "deadbeef" {
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
	if err := logEvent(scratchDir, "epic", Event{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}); err != nil {
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

	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, NewTextEventSink(io.Discard)), epics[0])
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
	if err := logEvent(scratchDir, "epic", Event{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}); err != nil {
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

	_, err = reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees", RepoDir: "/fake/repo"}, NewTextEventSink(io.Discard)), epics[0])
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

// TestReconcile_DoneTicketStaleCleanup_FinishesLeftoverCleanup exercises
// ticket 04: a done ticket classified doneStaleCleanup (commits already
// landed, but a leftover worktree/tab/branch survived a crash between marking
// done and the cleanup step right after it) gets that cleanup finished on
// startup — worktree removed, tab closed, and its now-redundant branch
// deleted.
func TestReconcile_DoneTicketStaleCleanup_FinishesLeftoverCleanup(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "# C\n\n**Status:** done\n",
	})
	if err := logEvent(scratchDir, "epic", Event{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}); err != nil {
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
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return true, nil } // commits landed
	d.WorktreeExists = func(path string) (bool, error) { return strings.Contains(path, "iter-03"), nil }
	// d.RevParse defaults to returning "deadbeef" for any ref (fakeDeps), so the
	// iteration branch is treated as still existing too.

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
	var deletedBranch string
	d.DeleteBranch = func(repoDir, branch string) error {
		deletedBranch = branch
		return nil
	}

	var out bytes.Buffer
	_, err = reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees", RepoDir: "/fake/repo"}, NewTextEventSink(&out)), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	if !strings.Contains(removedWorktree, "iter-03") {
		t.Errorf("removedWorktree = %q, want the leftover iter-03 worktree removed", removedWorktree)
	}
	if closedTab != "tab-iter-03" {
		t.Errorf("closedTab = %q, want the leftover iter-03 tab closed", closedTab)
	}
	if deletedBranch != "ralph-loop/iter-03" {
		t.Errorf("deletedBranch = %q, want ralph-loop/iter-03 deleted", deletedBranch)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "03-c.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "Status:** done") {
		t.Errorf("ticket status changed unexpectedly:\n%s", raw)
	}

	reports := strings.Split(out.String(), "\n")
	found := false
	for _, r := range reports {
		if strings.Contains(r, "ticket 03") && strings.Contains(r, "finished the interrupted cleanup") {
			found = true
		}
	}
	if !found {
		t.Errorf("reports = %v, want a report line for the finished cleanup", reports)
	}
}

// TestReconcile_DoneTicketFullyClean_NoOp verifies a done ticket with commits
// landed and nothing left behind (doneOK) is untouched: no worktree/tab/
// branch cleanup calls, no spurious report lines.
func TestReconcile_DoneTicketFullyClean_NoOp(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "# C\n\n**Status:** done\n",
	})
	if err := logEvent(scratchDir, "epic", Event{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return true, nil }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") } // branch gone

	cleanupCalled := false
	d.RemoveWorktree = func(repoDir, path string, force bool) error {
		cleanupCalled = true
		return nil
	}
	d.TabClose = func(tabID string) error {
		cleanupCalled = true
		return nil
	}
	d.DeleteBranch = func(repoDir, branch string) error {
		cleanupCalled = true
		return nil
	}

	var out bytes.Buffer
	_, err = reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees", RepoDir: "/fake/repo"}, NewTextEventSink(&out)), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	if cleanupCalled {
		t.Error("a fully-clean done ticket must not trigger any worktree/tab/branch cleanup call")
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want no spurious log lines for a fully-clean done ticket", out.String())
	}
}
