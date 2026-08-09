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
		WorkspaceID:  workspaceID,
		Paths:        paths,
		Agent:        AgentClaude,
		SmartZone:    defaultSmartZone,
		Gate:         NewGate(),
		FeatureLock:  &sync.Mutex{},
		WorktreeLock: &sync.Mutex{},
		Sink:         sink,
		Scope:        RunScope{wholeEpic: true},
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
		return []herdr.Tab{{Label: "epic-iter-01", WorkspaceID: workspaceID}}, nil
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

func TestReconcile_ClaimedWithLiveTab_TicketReattachedCarriesLiveSessionIdentity(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	if err := logEvent(scratchDir, "epic", Event{
		Type: eventIterationStarted, Ticket: "01", Agent: AgentClaude,
		AgentSession: "sess-1", Cwd: "/repo/epic-iter-01",
	}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID}}, nil
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

	if reattached.identifier != "01" || reattached.label != "epic-iter-01" || reattached.cwd != "/fake/worktrees/epic-item-01" || reattached.sessionID != "session-epic-iter-01" {
		t.Errorf("TicketReattached args = %+v, want live Herdr cwd/session identity", reattached)
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
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID}}, nil
	}

	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, NewTextEventSink(io.Discard)), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 1 || reattached[0].Number != 1 {
		t.Fatalf("reattached = %v, want needs-attention ticket 01", reattached)
	}
}

// TestReconcile_ClaimedWithLiveTabOutsideScope_NotReattached covers ticket
// 23's requirement that reconcile only reattaches iterations belonging to
// the requested run scope — a claimed ticket with a live tab left outside
// the scope belongs to a different (or not-yet-started) run and must be
// left exactly as found rather than reattached.
func TestReconcile_ClaimedWithLiveTabOutsideScope_NotReattached(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{Label: "epic-iter-01", WorkspaceID: workspaceID}}, nil
	}

	rp := testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, NewTextEventSink(io.Discard))
	// A scope that requested only ticket 99 — ticket 01 is outside it.
	rp.Scope = RunScope{data: &scopeData{ticketIDs: map[string]struct{}{"99": {}}}}

	reattached, err := reconcile(d, rp, epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Fatalf("reattached = %v, want none (ticket 01 is outside the scope)", reattached)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: claimed") {
		t.Errorf("out-of-scope ticket status changed unexpectedly:\n%s", raw)
	}
}

// TestReconcile_NeedsAttentionOutsideScope_NotReattached is the
// needs-attention counterpart of the claimed case above.
func TestReconcile_NeedsAttentionOutsideScope_NotReattached(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: needs-attention\ntype: task\n---\n# A\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}
	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID}}, nil
	}

	rp := testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, NewTextEventSink(io.Discard))
	rp.Scope = RunScope{data: &scopeData{ticketIDs: map[string]struct{}{"99": {}}}}

	reattached, err := reconcile(d, rp, epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Fatalf("reattached = %v, want none (needs-attention ticket 01 is outside the scope)", reattached)
	}
}

// TestRun_NeedsAttentionWithoutLiveTab_SchedulesOtherTicketsThenParks covers
// parking as a last resort: a needs-attention ticket with no live iteration
// left to reattach to is human-clearable, so it must not hold up the open
// ticket next to it — that one runs first, and only then does the run park.
func TestRun_NeedsAttentionWithoutLiveTab_SchedulesOtherTicketsThenParks(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-attention.md": "---\nid: \"01\"\nstatus: needs-attention\ntype: task\n---\n# Attention\n",
		"02-open.md":      "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Open\n",
	})
	d, prompts, _ := fakeDeps()
	d.TabList = func(string) ([]herdr.Tab, error) { return nil, nil }

	sink := &recordingSink{}
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v, want nil (the run parks on ticket 01)", err)
	}
	if len(*prompts) != 1 {
		t.Errorf("prompts = %v, want ticket 02 run before the park", *prompts)
	}
	if len(sink.parkedStalled) != 1 || len(sink.parkedStalled[0]) != 1 || sink.parkedStalled[0][0] != "01" {
		t.Errorf("EpicParked calls = %v, want one naming ticket 01", sink.parkedStalled)
	}
	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "02-open.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket 02 = %s, want it landed done before the park", raw)
	}
}

func TestRun_RestartedNeedsAttentionRecoversThenResumesScheduling(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-attention.md": "---\nid: \"01\"\nstatus: needs-attention\ntype: task\n---\n# Attention\n",
		"02-open.md":      "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Open\n",
	})
	d, prompts, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID}}, nil
	}
	var mu sync.Mutex
	sawClaimed := false
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		if strings.Contains(dir, "epic-item-01") {
			raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-attention.md"))
			mu.Lock()
			sawClaimed = err == nil && strings.Contains(string(raw), "claimed")
			mu.Unlock()
		}
		return 1, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&strings.Builder{})); err != nil {
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
	events, _, err := readEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	foundResumed := false
	for _, event := range events {
		if event.Type == eventResumed && event.Ticket == "01" && (event.Pane != "pane-epic-iter-01" || event.Tab != "tab-epic-iter-01" || event.Cwd != "/fake/worktrees/epic-item-01" || event.AgentSession != "session-epic-iter-01") {
			t.Errorf("resumed attribution = %+v, want original pane/tab/cwd/session", event)
		}
		foundResumed = foundResumed || event.Type == eventResumed && event.Ticket == "01"
	}
	if !foundResumed {
		t.Errorf("events = %v, want resumed event for reattached ticket", events)
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
