package ralphloop

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/herdr"
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
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
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
		if ref == iterBranch("epic", "01") {
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
	if !strings.Contains(string(raw), "status: open") {
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
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
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
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("orphaned claim with unlanded commits not recovered to done:\n%s", raw)
	}

	if len(*removed) != 1 {
		t.Errorf("removed worktrees = %v, want exactly one removal (cleanup only after landing the recovered commits)", *removed)
	}
}

// TestReconcile_ClaimedWithNoLiveTabButUnlandedCommits_ReportsRecoveringBeforeCherryPick
// covers the same UI gap as the doneRecoverable case above, but for an
// orphaned claim's synchronous re-cherry-pick: TicketRecovering must fire
// before CherryPickStarted so a renderer has a live row to update instead of
// showing nothing while the recovery runs.
func TestReconcile_ClaimedWithNoLiveTabButUnlandedCommits_ReportsRecoveringBeforeCherryPick(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }

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
		t.Fatalf("calls = %v, want TicketRecovering to fire before the orphaned claim's cherry-pick", calls)
	}
	if cherryPickIdx != -1 && recoveringIdx > cherryPickIdx {
		t.Errorf("calls = %v, want TicketRecovering before CherryPickStarted", calls)
	}
}

func TestReconcile_ClaimedWithLiveTab_ReturnsReattached(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
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
	if !strings.Contains(string(raw), "status: claimed") {
		t.Errorf("ticket status changed unexpectedly:\n%s", raw)
	}
}

// reattachSink embeds occupancySink (noopEventSink underneath) and
// additionally hooks TicketReattached, for asserting the cwd/sessionID it
// carries plus the immediate ContextOccupancy read it triggers.
type reattachSink struct {
	*occupancySink
	onTicketReattached func(identifier, label, cwd, sessionID string)
}

func (s *reattachSink) TicketReattached(identifier, label, cwd, sessionID string) {
	if s.onTicketReattached != nil {
		s.onTicketReattached(identifier, label, cwd, sessionID)
	}
}

// TestReconcile_ClaimedWithLiveTab_TicketReattachedCarriesCwdAndSessionIDPlusImmediateOccupancy
// covers ticket 02's reattach-time requirements: TicketReattached must carry
// the cwd/sessionID recovered from the run log's last iteration-started
// event, and trigger one immediate ContextOccupancy read/emit — the same two
// signals launchAndPrompt fires at fresh-start time (see launch_test.go),
// but sourced from the run log instead of a just-launched agent.Session.
func TestReconcile_ClaimedWithLiveTab_TicketReattachedCarriesCwdAndSessionIDPlusImmediateOccupancy(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	if err := logEvent(scratchDir, "epic", Event{
		Type: eventIterationStarted, Ticket: "01", Agent: AgentClaude,
		AgentSession: "sess-1", Cwd: "/repo/iter-01",
	}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}
	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		return 4200, true, nil
	}

	var reattached struct{ identifier, label, cwd, sessionID string }
	sink := &reattachSink{
		occupancySink: &occupancySink{},
		onTicketReattached: func(identifier, label, cwd, sessionID string) {
			reattached.identifier, reattached.label, reattached.cwd, reattached.sessionID = identifier, label, cwd, sessionID
		},
	}

	_, err = reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, sink), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}

	if reattached.identifier != "01" || reattached.label != "iter-01" || reattached.cwd != "/repo/iter-01" || reattached.sessionID != "sess-1" {
		t.Errorf("TicketReattached args = %+v, want {01 iter-01 /repo/iter-01 sess-1}", reattached)
	}
	if len(sink.occupancySink.calls) != 1 || sink.occupancySink.calls[0].identifier != "01" || sink.occupancySink.calls[0].tokens != 4200 {
		t.Errorf("ContextOccupancy calls = %+v, want one {01 4200} for the immediate reattach-time read", sink.occupancySink.calls)
	}
}

func TestReconcile_NeedsAttentionWithLiveTab_ReturnsReattached(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: needs-attention\ntype: task\n---\n# A\n",
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
		"01-attention.md": "---\nid: \"01\"\nstatus: needs-attention\ntype: task\n---\n# Attention\n",
		"02-open.md":      "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Open\n",
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
		"01-attention.md": "---\nid: \"01\"\nstatus: needs-attention\ntype: task\n---\n# Attention\n",
		"02-open.md":      "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Open\n",
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
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: done\ntype: task\n---\n# B\n",
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
