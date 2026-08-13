package ralphloop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/transcript"
)

// TestRun_RestartWithClaimedTicketButNoLiveTab_RevertsAndRerunsFromScratch
// exercises the full Run() path (not just reconcile in isolation): a ticket
// left claimed by a prior crashed invocation, with nothing live in the herdr
// workspace, is reverted to open and then picked up and run fresh by normal
// scheduling.
func TestRun_RestartWithClaimedTicketButNoLiveTab_RerunsFromScratch(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	d, prompts, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return nil, nil
	}
	// No iteration branch was ever created for this claim, so reconcile has
	// nothing to recover — only the plain revert-then-rerun applies.
	d.RevParse = func(dir, ref string) (string, error) {
		if ref == iterBranch("epic", "01") {
			return "", fmt.Errorf("unknown revision")
		}
		return "deadbeef", nil
	}
	origAgentPrompt := d.AgentPrompt
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		agent, err := origAgentPrompt(opts)
		agent.AgentSession = "session-fresh-01"
		return agent, err
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*prompts) != 1 || !strings.HasSuffix((*prompts)[0], "01-a.md") {
		t.Fatalf("prompts = %v, want ticket 01 run fresh", *prompts)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket not landed after restart:\n%s", raw)
	}

	ticket, err := schema.ParseTicket(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("schema.ParseTicket: %v", err)
	}
	if want := []string{"session-fresh-01"}; len(ticket.SessionIDs) != 1 || ticket.SessionIDs[0] != want[0] {
		t.Errorf("SessionIDs = %v, want %v", ticket.SessionIDs, want)
	}
}

// TestRun_RestartWithClaimedTicketAndLiveTab_ReattachesWithoutReplayingPrompt
// verifies the other half of ticket 08: a claimed ticket whose iter-NN tab is
// still alive is reattached (no fresh worktree create, no initial prompt
// replayed) and driven through to completion.
func TestRun_RestartWithClaimedTicketAndLiveTab_ReattachesWithoutReplayingPrompt(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	d, prompts, removed := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID}}, nil
	}

	var worktreeCreateCalledForIter bool
	origAddWorktree := d.AddWorktree
	d.AddWorktree = func(repoDir, path, branch, base string) error {
		if strings.Contains(path, "epic-item-01") {
			worktreeCreateCalledForIter = true
		}
		return origAddWorktree(repoDir, path, branch, base)
	}

	if err := Run(RunOptions{EpicName: "epic", Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
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
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("reattached ticket not marked done:\n%s", raw)
	}

	ticket, err := schema.ParseTicket(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("schema.ParseTicket: %v", err)
	}
	if want := []string{"session-epic-iter-01"}; len(ticket.SessionIDs) != 1 || ticket.SessionIDs[0] != want[0] {
		t.Errorf("SessionIDs = %v, want %v (reattach appends the live agent's session)", ticket.SessionIDs, want)
	}

	events, _, err := readEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	foundFinish := false
	for _, event := range events {
		if event.Type == eventIterationFinished && (event.Pane != "pane-epic-iter-01" || event.Tab != "tab-epic-iter-01" || event.Cwd != "/fake/worktrees/epic-item-01" || event.AgentSession != "session-epic-iter-01") {
			t.Errorf("iteration-finished attribution = %+v, want original pane/tab/cwd/session", event)
		}
		foundFinish = foundFinish || event.Type == eventIterationFinished
	}
	if !foundFinish {
		t.Errorf("events = %v, want iteration-finished", events)
	}
}

// TestRun_RestartWithNeedsRepairTicketAndLiveResolver_ReattachesWithoutReforking
// covers the needs-repair retry path's counterpart to conflict-lifecycle/
// 02a's guard: a needs-repair ticket is reattached to its still-live
// iter-NN tab (like the claimed-ticket case above) and driven through
// reattachIteration -> finishIteration -> landCherryPick ->
// cherryPickWithConflictResolution. That cherry-pick finds the sequencer
// already mid-conflict, with the conflict-resolution agent forked for it
// still live in a second, stray tab from before the crash. It must reattach
// to that live resolver instead of aborting the stale sequencer state and
// forking a second one under the same conflict-labeled tab.
func TestRun_RestartWithNeedsRepairTicketAndLiveResolver_ReattachesWithoutReforking(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md":                    "---\nid: \"01\"\nstatus: needs-repair\ntype: task\n---\n# A\n",
		"01a-conflict-resolution.md": "---\nid: \"01a\"\nstatus: claimed\ntype: conflict-resolution\nparent: \"01\"\n---\n# Conflict resolution for 01\n",
	})
	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{
			{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID},
			{TabID: "tab-conflict-01", Label: "conflict-01", WorkspaceID: workspaceID},
		}, nil
	}

	// The sequencer already owns a conflict from before the crash — no
	// CherryPickRange call for this ticket ever produces it. The AgentWait
	// hook (fired once reattachLiveConflictResolver waits out the
	// reattached resolver in its own "pane-conflict-01" pane) flips it to
	// resolved.
	inProgress := true
	d.CherryPickInProgress = func(dir string) (bool, error) { return inProgress, nil }
	origAgentWait := d.AgentWait
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		if opts.Target == "pane-conflict-01" {
			inProgress = false
		}
		return origAgentWait(opts)
	}

	var picks, aborts int32
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		atomic.AddInt32(&picks, 1)
		return nil
	}
	d.AbortCherryPick = func(dir string) error {
		atomic.AddInt32(&aborts, 1)
		return nil
	}

	var mu sync.Mutex
	var conflictTabCreates int
	origTabCreate := d.TabCreate
	d.TabCreate = func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error) {
		if strings.HasPrefix(opts.Label, "conflict-") {
			mu.Lock()
			conflictTabCreates++
			mu.Unlock()
		}
		return origTabCreate(opts)
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if atomic.LoadInt32(&aborts) != 0 {
		t.Errorf("AbortCherryPick calls = %d, want 0 (must reattach the live resolver, not abort its sequencer state)", aborts)
	}
	if conflictTabCreates != 0 {
		t.Errorf("conflict-labeled TabCreate calls = %d, want 0 (must reuse the live resolver's tab, not fork a second one)", conflictTabCreates)
	}
	if atomic.LoadInt32(&picks) != 0 {
		t.Errorf("CherryPickRange calls = %d, want 0 (the sequencer was already mid-conflict)", picks)
	}

	childRaw := findConflictResolutionChild(t, scratchDir, "epic")
	if !strings.Contains(childRaw, "status: done") {
		t.Errorf("conflict-resolution child ticket = %q, want status: done once the reattached resolver finished", childRaw)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("parent ticket = %q, want status: done", raw)
	}
}

// TestRun_RestartWithClaimedTicketAlreadyIdle_SkipsWaitAndCherryPicks
// verifies ticket 05: when a reattached iteration's pane already shows
// idle/done in TabList at reattach time (the agent finished while the
// previous invocation was down), the reattach proceeds straight to
// cherry-pick without ever polling AgentWait — a pane already sitting idle
// with no further status transition coming would otherwise wait out every
// poll timeout forever.
// TestRun_ReattachClearsStaleIterationStatusBeforeFinish verifies 02b: a
// ticket that stays claimed throughout a reattach (the common case, never
// routing through Claim) still must not carry a pre-restart iteration_status
// report into the new attach. TabList is the first Deps call
// reattachIteration makes after computing the ticket's iteration paths, so
// hooking it to snapshot the ticket's on-disk iteration_status proves the
// clear ran before finishIteration/waitForFinish, not just by the time Run
// returns.
func TestRun_ReattachClearsStaleIterationStatusBeforeFinish(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\niteration_status: finished\ntype: task\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	d, _, _ := fakeDeps()
	var iterationStatusAtTabList string
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		ticket, err := schema.ParseTicket(ticketPath)
		if err != nil {
			t.Fatalf("schema.ParseTicket at TabList: %v", err)
		}
		iterationStatusAtTabList = string(ticket.IterationStatus)
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID}}, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if iterationStatusAtTabList != "" {
		t.Errorf("iteration_status at TabList time = %q, want already cleared before the live-tab lookup", iterationStatusAtTabList)
	}

	ticket, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatalf("schema.ParseTicket: %v", err)
	}
	if ticket.IterationStatus != "" {
		t.Errorf("final IterationStatus = %q, want cleared", ticket.IterationStatus)
	}
}

func TestRun_RestartWithClaimedTicketAlreadyIdle_SkipsWaitAndCherryPicks(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	d, _, removed := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID, AgentStatus: "idle"}}, nil
	}
	d.AgentGet = func(string) (herdr.Agent, error) {
		return herdr.Agent{PaneID: "pane-epic-iter-01", WorkspaceID: "ws1", TabID: "tab-epic-iter-01", AgentStatus: "idle", AgentSession: "session-epic-iter-01"}, nil
	}

	var agentWaitCalls int
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		agentWaitCalls++
		return herdr.Agent{AgentStatus: "idle"}, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if agentWaitCalls != 1 {
		t.Errorf("AgentWait calls = %d, want 1 (confirmFinished's debounce re-check) for a pane already idle at reattach, not waitForFinish's full poll loop", agentWaitCalls)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the reattached iteration's worktree removed on completion", *removed)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
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

func TestRun_ReattachedCodexSessionIdentityFailureStopsSafely(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		agent    herdr.Agent
		verified bool
		wantErr  string
	}{
		{name: "missing session", agent: herdr.Agent{PaneID: "pane-epic-iter-01", WorkspaceID: "ws1", TabID: "tab-epic-iter-01", AgentStatus: "working"}, wantErr: "missing live Codex session"},
		{name: "mismatched rollout", agent: herdr.Agent{PaneID: "pane-epic-iter-01", WorkspaceID: "ws1", TabID: "tab-epic-iter-01", AgentStatus: "working", AgentSession: "wrong-session"}, wantErr: "does not match rollout metadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scratchDir := writeEpic(t, "epic", map[string]string{
				"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
			})
			d, prompts, removed := fakeDeps()
			d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
				return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID}}, nil
			}
			d.AgentGet = func(string) (herdr.Agent, error) { return tc.agent, nil }
			d.VerifyCodexSession = func(cwd, sessionID string) (bool, error) { return tc.verified, nil }
			var starts int
			d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
				starts++
				return herdr.Agent{}, nil
			}

			// The failed reattach leaves the epic's only ticket
			// needs-repair, so the run parks on it.
			runUntilParked(t, RunOptions{EpicName: "epic", Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})

			raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if !strings.Contains(string(raw), "status: needs-repair") || !strings.Contains(string(raw), tc.wantErr) {
				t.Errorf("ticket after failed reattach:\n%s\nwant needs-repair with %q", raw, tc.wantErr)
			}
			if starts != 0 || len(*prompts) != 0 || len(*removed) != 0 {
				t.Errorf("side effects: starts=%d prompts=%v removed=%v, want none", starts, *prompts, *removed)
			}
		})
	}
}

func TestRun_ReattachedCloseUsesLiveSessionInsteadOfStaleRunLog(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	if err := logEvent(scratchDir, "epic", Event{
		Type:         eventIterationStarted,
		Ticket:       "01",
		Agent:        AgentClaude,
		AgentSession: "sess-original",
		Cwd:          "/fake/worktrees/epic-iter-01",
	}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID}}, nil
	}
	d.AgentGet = func(string) (herdr.Agent, error) {
		return herdr.Agent{PaneID: "pane-epic-iter-01", WorkspaceID: "ws1", TabID: "tab-epic-iter-01", AgentStatus: "working", AgentSession: "sess-live"}, nil
	}
	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		if cwd == "/fake/worktrees/epic-item-01" && sessionID == "sess-live" {
			return 54321, true, nil
		}
		return 0, false, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := mustParse(t, filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if got.Status != schema.StatusDone {
		t.Errorf("Status = %q, want done", got.Status)
	}
	if got.ActualContextWindow != 54321 {
		t.Errorf("ActualContextWindow = %d, want 54321 from the live session", got.ActualContextWindow)
	}
}

// TestRun_ReattachedCommitlessCloseWithNoLiveSession verifies ticket 08: the
// commitless close (iteration_status: finished + commitless: true + zero
// commits reaches done with no cherry-pick) applies identically on the
// reattached path, even when the reattach recovers no live session id of its
// own (AgentSession == "") — the route ticket 08 calls out as most likely to
// be missed because it's reached only after a restart.
func TestRun_ReattachedCommitlessCloseWithNoLiveSession(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\niteration_status: finished\ncommitless: true\ntype: task\n---\n# A\n",
	})
	d, _, removed := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID, AgentStatus: "idle"}}, nil
	}
	d.AgentGet = func(string) (herdr.Agent, error) {
		return herdr.Agent{PaneID: "pane-epic-iter-01", WorkspaceID: "ws1", TabID: "tab-epic-iter-01", AgentStatus: "idle", AgentSession: ""}, nil
	}
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	var cherryPickCalls int
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		cherryPickCalls++
		return nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if cherryPickCalls != 0 {
		t.Errorf("CherryPickRange calls = %d, want 0 for a commitless close", cherryPickCalls)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the reattached iteration's worktree cleaned up", *removed)
	}

	got := mustParse(t, filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if got.Status != schema.StatusDone {
		t.Errorf("Status = %q, want done", got.Status)
	}

	events, _, err := readEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	foundCommitless := false
	for _, e := range events {
		if e.Type == eventCherryPicked {
			t.Errorf("events = %v, want no cherry-picked event for a commitless close", events)
		}
		if e.Type == eventCommitless && e.Ticket == "01" {
			foundCommitless = true
		}
	}
	if !foundCommitless {
		t.Errorf("events = %v, want a commitless event for ticket 01", events)
	}
}

// TestRun_ReattachedClose_NoPriorSessionInLog_OmitsMetadata verifies the
// ticket 06a edge case: when the run log has no iteration-started event to
// recover a session id from, the reattached close still marks the ticket
// done, without writing a wrong/placeholder actual_context_window.
func TestRun_ReattachedClose_NoPriorSessionInLog_OmitsMetadata(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID}}, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := mustParse(t, filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if got.Status != schema.StatusDone {
		t.Errorf("Status = %q, want done", got.Status)
	}
	if got.ActualContextWindow != 0 {
		t.Errorf("ActualContextWindow = %d, want 0 (no prior session to backfill from)", got.ActualContextWindow)
	}
}

// idleReattachDeps returns fakeDeps() wired for a reattach whose live pane
// already reports idle at AgentGet time, the alreadyFinished short-circuit's
// entry condition. AgentWait always answers idle too, so once the gate lets
// the short-circuit through, nothing loops back into waitForFinish's full
// poll.
func idleReattachDeps(t *testing.T, agentSession string) (Deps, *[]string) {
	t.Helper()
	d, _, removed := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID, AgentStatus: "idle"}}, nil
	}
	d.AgentGet = func(string) (herdr.Agent, error) {
		return herdr.Agent{PaneID: "pane-epic-iter-01", WorkspaceID: "ws1", TabID: "tab-epic-iter-01", AgentStatus: "idle", AgentSession: agentSession}, nil
	}
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}
	return d, removed
}

// TestRun_ReattachAlreadyIdle_BackgroundTaskOutstanding_HoldsShortCircuit
// covers ticket 04's core fix: the alreadyFinished short-circuit must consult
// the same background-task gate confirmFinished/waitForBackgroundTasks apply
// in waitForFinish, not skip it - this is the mechanism both real incidents
// looped through (false park -> reopen -> reattach -> immediate re-finish).
func TestRun_ReattachAlreadyIdle_BackgroundTaskOutstanding_HoldsShortCircuit(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	d, removed := idleReattachDeps(t, "session-epic-iter-01")
	var reads int
	d.ReadBackgroundTasks = func(cwd, sessionID string) (transcript.BackgroundTaskReading, error) {
		reads++
		status := transcript.BackgroundTaskOutstandingFresh
		if reads >= 2 {
			status = transcript.BackgroundTaskResolved
		}
		return transcript.BackgroundTaskReading{
			Markers: []transcript.BackgroundTaskMarker{{TaskID: "task-1", Status: status}},
		}, nil
	}
	d.Sleep = func(time.Duration) {}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if reads < 2 {
		t.Errorf("ReadBackgroundTasks calls = %d, want at least 2: the gate must be consulted and held until the marker resolves", reads)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the reattached iteration's worktree cleaned up once the gate releases", *removed)
	}

	events, _, err := readEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	var held bool
	for _, e := range events {
		if e.Type == eventBackgroundTaskGateHeld {
			held = true
		}
	}
	if !held {
		t.Errorf("events = %v, want a background-task-gate-held event for the reattach short-circuit", events)
	}
}

// TestRun_ReattachAlreadyIdle_NoBackgroundTask_DebouncesBeforeShortCircuit
// covers the bonus fix: even with no background task involved at all, the
// short-circuit must debounce a just-observed idle pane before trusting it -
// a genuine mid-turn pause (agent still working, nothing backgrounded) must
// not be misread as finished. Here the debounce re-check itself catches the
// agent still working, so the short-circuit is not taken and control falls
// through to waitForFinish's normal poll loop instead.
func TestRun_ReattachAlreadyIdle_NoBackgroundTask_DebouncesBeforeShortCircuit(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	d, removed := idleReattachDeps(t, "session-epic-iter-01")
	var waitCalls int
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waitCalls++
		if waitCalls == 1 {
			// confirmFinished's debounce re-check: the agent is still working,
			// so the just-observed idle at AgentGet time was a transient blip.
			return herdr.Agent{}, errors.New("timed out waiting for agent status")
		}
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}
	d.Sleep = func(time.Duration) {}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if waitCalls < 2 {
		t.Errorf("AgentWait calls = %d, want at least 2: debounce re-check plus waitForFinish's own poll after the short-circuit is declined", waitCalls)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the reattached iteration's worktree cleaned up once it genuinely finishes", *removed)
	}

	got := mustParse(t, filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if got.Status != schema.StatusDone {
		t.Errorf("Status = %q, want done", got.Status)
	}
}

// TestRun_ReattachAlreadyIdle_EmptySession_FallsBackToTicketSessionIDs
// covers ticket 04's session-id recovery: when AgentGet's live AgentSession
// is empty (the Claude case at reattach), the background-task gate must
// still be consulted using a session id recovered from the ticket's own
// session_ids frontmatter, not skipped for want of a session.
func TestRun_ReattachAlreadyIdle_EmptySession_FallsBackToTicketSessionIDs(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\nsession_ids:\n    - \"sess-older\"\n    - \"sess-prior\"\n---\n# A\n",
	})
	d, removed := idleReattachDeps(t, "")
	var gotSessionID string
	d.ReadBackgroundTasks = func(cwd, sessionID string) (transcript.BackgroundTaskReading, error) {
		gotSessionID = sessionID
		return transcript.BackgroundTaskReading{}, nil
	}
	d.Sleep = func(time.Duration) {}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if gotSessionID != "sess-prior" {
		t.Errorf("session id consulted by the gate = %q, want the ticket's most recent recorded session_ids entry %q", gotSessionID, "sess-prior")
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the reattached iteration's worktree cleaned up", *removed)
	}
}

// TestRun_ReattachAlreadyIdle_EmptySessionAndNoSessionIDs_FallsBackToRunLog
// covers the last link in ticket 04's session-id recovery chain: with both
// AgentGet's AgentSession and the ticket's session_ids empty, the gate falls
// back to the run log's last iteration-started event for this ticket.
func TestRun_ReattachAlreadyIdle_EmptySessionAndNoSessionIDs_FallsBackToRunLog(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	if err := logEvent(scratchDir, "epic", Event{
		Type:         eventIterationStarted,
		Ticket:       "01",
		Agent:        AgentClaude,
		AgentSession: "sess-from-log",
		Cwd:          "/fake/worktrees/epic-item-01",
	}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}

	d, removed := idleReattachDeps(t, "")
	var gotSessionID string
	d.ReadBackgroundTasks = func(cwd, sessionID string) (transcript.BackgroundTaskReading, error) {
		gotSessionID = sessionID
		return transcript.BackgroundTaskReading{}, nil
	}
	d.Sleep = func(time.Duration) {}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if gotSessionID != "sess-from-log" {
		t.Errorf("session id consulted by the gate = %q, want the run log's last iteration-started session %q", gotSessionID, "sess-from-log")
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the reattached iteration's worktree cleaned up", *removed)
	}
}
