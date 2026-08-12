package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// TestReconcile_DoneTicketRecoverable_AutoRecherryPicksAndReports exercises
// ticket 02: a done ticket classified doneRecoverable (its landed commit
// missing from the feature branch, but its iteration branch still holds it)
// gets re-cherry-picked automatically, with a cherry-picked event logged and
// a report line naming what was restored.
func TestReconcile_DoneTicketRecoverable_AutoRecherryPicksAndReports(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "---\nid: \"03\"\nstatus: done\ntype: task\n---\n# C\n",
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

	sink := newRecordingEventSink()
	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, sink), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Errorf("reattached = %v, want none for a done ticket", reattached)
	}
	if len(picked) != 1 {
		t.Fatalf("CherryPickRange calls = %v, want exactly one re-cherry-pick", picked)
	}

	if !hasEvent(sink, LiveEventTicketRecovered, func(ev LiveEvent) bool { return ev.Identifier == "03" }) {
		t.Errorf("events = %+v, want a recovered event naming ticket 03 as restored", sink.Events())
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

// TestReconcile_DoneTicketRecoverable_ReportsRecoveringBeforeCherryPick
// covers a UI bug: unlike a normal iteration (LiveEventIterationStarted
// seeds a live row before CherryPickStarted), a startup repair went straight
// to CherryPickStarted/ConflictResolutionStarted with no live row to update,
// so the tickets tab showed nothing (not even a spinner) while a done
// ticket's commits were being re-landed. TicketRecovering must fire first so
// a renderer has something to attach the cherry-pick phase to.
func TestReconcile_DoneTicketRecoverable_ReportsRecoveringBeforeCherryPick(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "---\nid: \"03\"\nstatus: done\ntype: task\n---\n# C\n",
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
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error { return nil }

	sink := &recordingSink{}
	_, err = reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, sink), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	calls := sink.snapshot()
	recoveringIdx, cherryPickIdx := -1, -1
	for i, c := range calls {
		if c == "TicketRecovering" {
			recoveringIdx = i
		}
		if c == "CherryPickStarted" {
			cherryPickIdx = i
		}
	}
	if recoveringIdx == -1 {
		t.Fatalf("calls = %v, want TicketRecovering to fire before the repair's cherry-pick", calls)
	}
	if cherryPickIdx != -1 && recoveringIdx > cherryPickIdx {
		t.Errorf("calls = %v, want TicketRecovering before CherryPickStarted", calls)
	}
}

// TestReconcile_DoneTicketRecoverable_ConflictGoesThroughResolutionPath
// verifies the repair's re-cherry-pick reuses the exact same conflict-
// resolution path (launching a "/gx-resolving-merge-conflicts" agent in the
// feature worktree) a normal iteration's first cherry-pick uses, rather than
// a separate repair-specific conflict handler.
func TestReconcile_DoneTicketRecoverable_ConflictGoesThroughResolutionPath(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "---\nid: \"03\"\nstatus: done\ntype: task\n---\n# C\n",
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
		if opts.Text == "/gx-resolving-merge-conflicts" {
			resolutionPrompted = true
			inProgress = false
		}
		return origAgentPrompt(opts)
	}

	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, noopEventSink{}), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Errorf("reattached = %v, want none for a done ticket", reattached)
	}
	if !resolutionPrompted {
		t.Error("expected a /gx-resolving-merge-conflicts agent to be prompted on cherry-pick conflict during repair")
	}
}

// TestReconcile_DoneTicketRecoverable_ReattachesLiveConflictResolverWithoutReforking
// covers the doneRecoverable repair path's counterpart to conflict-lifecycle/
// 02a's guard: the repair's re-cherry-pick finds the sequencer already
// mid-conflict (from before the crash) with the conflict-resolution agent
// that was forked for it still live in its own tab. It must reattach to
// that live resolver instead of aborting the stale sequencer state and
// forking a second one under the same conflict-labeled tab. Unlike the
// genuine-new-conflict precedent above (no live resolver, TabList nil),
// this exercises reattachLiveConflictResolver, which never calls
// AgentPrompt, so the in-progress flag flips via an AgentWait hook instead.
func TestReconcile_DoneTicketRecoverable_ReattachesLiveConflictResolverWithoutReforking(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md":                    "---\nid: \"03\"\nstatus: done\ntype: task\n---\n# C\n",
		"03a-conflict-resolution.md": "---\nid: \"03a\"\nstatus: claimed\ntype: conflict-resolution\nparent: \"03\"\n---\n# Conflict resolution for 03\n",
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
		return []herdr.Tab{{TabID: "tab-conflict-03", Label: "conflict-03", WorkspaceID: workspaceID}}, nil
	}
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil } // landed SHA missing

	// The sequencer already owns a conflict from before the crash. The
	// AgentWait hook (fired once reattachLiveConflictResolver waits out the
	// reattached resolver) flips it to resolved, mirroring what
	// AgentPrompt does for a fresh fork in the genuine-conflict precedent.
	inProgress := true
	d.CherryPickInProgress = func(dir string) (bool, error) { return inProgress, nil }
	origAgentWait := d.AgentWait
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		if opts.Target == "pane-conflict-03" {
			inProgress = false
		}
		return origAgentWait(opts)
	}

	var aborted bool
	d.AbortCherryPick = func(dir string) error {
		aborted = true
		return nil
	}
	var conflictTabCreates int
	origTabCreate := d.TabCreate
	d.TabCreate = func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error) {
		if strings.HasPrefix(opts.Label, "conflict-") {
			conflictTabCreates++
		}
		return origTabCreate(opts)
	}

	_, err = reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, noopEventSink{}), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	if aborted {
		t.Error("AbortCherryPick called, want the live conflict-resolution resolver reattached instead")
	}
	if conflictTabCreates != 0 {
		t.Errorf("conflict-labeled TabCreate calls = %d, want 0 (must reuse the live resolver's tab, not fork a second one)", conflictTabCreates)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "03-c.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket = %q, want status: done", raw)
	}
}

// TestReconcile_DoneTicketRecoverable_CleansUpLeftoverWorktreeAndTab verifies
// that once a doneRecoverable ticket's commits are repaired, its leftover
// iteration worktree/tab (if the crash left any behind) are removed/closed —
// branch deletion is left to a later ticket.
func TestReconcile_DoneTicketRecoverable_CleansUpLeftoverWorktreeAndTab(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "---\nid: \"03\"\nstatus: done\ntype: task\n---\n# C\n",
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
		return []herdr.Tab{{TabID: "tab-iter-03", Label: iterLabel("epic", "03"), WorkspaceID: workspaceID}}, nil
	}
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.WorktreeExists = func(path string) (bool, error) { return strings.Contains(path, "item-03"), nil }

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

	_, err = reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees", RepoDir: "/fake/repo"}, noopEventSink{}), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	if !strings.Contains(removedWorktree, "item-03") {
		t.Errorf("removedWorktree = %q, want the leftover item-03 worktree removed", removedWorktree)
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
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "---\nid: \"03\"\nstatus: done\ntype: task\n---\n# C\n",
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
		return []herdr.Tab{{TabID: "tab-iter-03", Label: iterLabel("epic", "03"), WorkspaceID: workspaceID}}, nil
	}
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return true, nil } // commits landed
	d.WorktreeExists = func(path string) (bool, error) { return strings.Contains(path, "item-03"), nil }
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

	sink := newRecordingEventSink()
	_, err = reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees", RepoDir: "/fake/repo"}, sink), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	if !strings.Contains(removedWorktree, "item-03") {
		t.Errorf("removedWorktree = %q, want the leftover item-03 worktree removed", removedWorktree)
	}
	if closedTab != "tab-iter-03" {
		t.Errorf("closedTab = %q, want the leftover iter-03 tab closed", closedTab)
	}
	if deletedBranch != "ralph-loop/epic-item-03" {
		t.Errorf("deletedBranch = %q, want ralph-loop/epic-item-03 deleted", deletedBranch)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "03-c.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket status changed unexpectedly:\n%s", raw)
	}

	if !hasEvent(sink, LiveEventTicketCleanupFinished, func(ev LiveEvent) bool { return ev.Identifier == "03" }) {
		t.Errorf("events = %+v, want a cleanup-finished event for ticket 03", sink.Events())
	}
}

// TestReconcile_DoneTicketFullyClean_NoOp verifies a done ticket with commits
// landed and nothing left behind (doneOK) is untouched: no worktree/tab/
// branch cleanup calls, no spurious report lines.
func TestReconcile_DoneTicketFullyClean_NoOp(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "---\nid: \"03\"\nstatus: done\ntype: task\n---\n# C\n",
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

	sink := newRecordingEventSink()
	_, err = reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees", RepoDir: "/fake/repo"}, sink), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	if cleanupCalled {
		t.Error("a fully-clean done ticket must not trigger any worktree/tab/branch cleanup call")
	}
	if len(sink.Events()) != 0 {
		t.Errorf("events = %+v, want no spurious events for a fully-clean done ticket", sink.Events())
	}
}
