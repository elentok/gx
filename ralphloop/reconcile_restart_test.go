package ralphloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets/schema"
)

// TestRun_RestartWithClaimedTicketButNoLiveTab_RevertsAndRerunsFromScratch
// exercises the full Run() path (not just reconcile in isolation): a ticket
// left claimed by a prior crashed invocation, with nothing live in the herdr
// workspace, is reverted to open and then picked up and run fresh by normal
// scheduling.
func TestRun_RestartWithClaimedTicketButNoLiveTab_RerunsFromScratch(t *testing.T) {
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
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket not landed after restart:\n%s", raw)
	}
}

// TestRun_RestartWithClaimedTicketAndLiveTab_ReattachesWithoutReplayingPrompt
// verifies the other half of ticket 08: a claimed ticket whose iter-NN tab is
// still alive is reattached (no fresh worktree create, no initial prompt
// replayed) and driven through to completion.
func TestRun_RestartWithClaimedTicketAndLiveTab_ReattachesWithoutReplayingPrompt(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	d, prompts, removed := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-iter-01", Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}

	var worktreeCreateCalledForIter bool
	origAddWorktree := d.AddWorktree
	d.AddWorktree = func(repoDir, path, branch, base string) error {
		if strings.Contains(path, "epic-item-01") {
			worktreeCreateCalledForIter = true
		}
		return origAddWorktree(repoDir, path, branch, base)
	}

	var out strings.Builder
	if err := Run(RunOptions{EpicName: "epic", Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
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

	events, _, err := readEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	foundFinish := false
	for _, event := range events {
		if event.Type == eventIterationFinished && (event.Pane != "pane-iter-01" || event.Tab != "tab-iter-01" || event.Cwd != "/fake/worktrees/epic-item-01" || event.AgentSession != "session-iter-01") {
			t.Errorf("iteration-finished attribution = %+v, want original pane/tab/cwd/session", event)
		}
		foundFinish = foundFinish || event.Type == eventIterationFinished
	}
	if !foundFinish {
		t.Errorf("events = %v, want iteration-finished", events)
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
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	d, _, removed := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-iter-01", Label: "iter-01", WorkspaceID: workspaceID, AgentStatus: "idle"}}, nil
	}
	d.AgentGet = func(string) (herdr.Agent, error) {
		return herdr.Agent{PaneID: "pane-iter-01", WorkspaceID: "ws1", TabID: "tab-iter-01", AgentStatus: "idle", AgentSession: "session-iter-01"}, nil
	}

	var agentWaitCalls int
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		agentWaitCalls++
		return herdr.Agent{AgentStatus: "idle"}, nil
	}

	var out strings.Builder
	if err := Run(RunOptions{EpicName: "epic", Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
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
	for _, tc := range []struct {
		name     string
		agent    herdr.Agent
		verified bool
		wantErr  string
	}{
		{name: "missing session", agent: herdr.Agent{PaneID: "pane-iter-01", WorkspaceID: "ws1", TabID: "tab-iter-01", AgentStatus: "working"}, wantErr: "missing live Codex session"},
		{name: "mismatched rollout", agent: herdr.Agent{PaneID: "pane-iter-01", WorkspaceID: "ws1", TabID: "tab-iter-01", AgentStatus: "working", AgentSession: "wrong-session"}, wantErr: "does not match rollout metadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scratchDir := writeEpic(t, "epic", map[string]string{
				"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
			})
			d, prompts, removed := fakeDeps()
			d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
				return []herdr.Tab{{TabID: "tab-iter-01", Label: "iter-01", WorkspaceID: workspaceID}}, nil
			}
			d.AgentGet = func(string) (herdr.Agent, error) { return tc.agent, nil }
			d.VerifyCodexSession = func(cwd, sessionID string) (bool, error) { return tc.verified, nil }
			var starts int
			d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
				starts++
				return herdr.Agent{}, nil
			}

			if err := Run(RunOptions{EpicName: "epic", Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&strings.Builder{})); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if !strings.Contains(string(raw), "status: needs-attention") || !strings.Contains(string(raw), tc.wantErr) {
				t.Errorf("ticket after failed reattach:\n%s\nwant needs-attention with %q", raw, tc.wantErr)
			}
			if starts != 0 || len(*prompts) != 0 || len(*removed) != 0 {
				t.Errorf("side effects: starts=%d prompts=%v removed=%v, want none", starts, *prompts, *removed)
			}
		})
	}
}

func TestRun_ReattachedCloseUsesLiveSessionInsteadOfStaleRunLog(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
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
	d.AgentGet = func(string) (herdr.Agent, error) {
		return herdr.Agent{PaneID: "pane-iter-01", WorkspaceID: "ws1", TabID: "tab-iter-01", AgentStatus: "working", AgentSession: "sess-live"}, nil
	}
	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		if cwd == "/fake/worktrees/epic-item-01" && sessionID == "sess-live" {
			return 54321, true, nil
		}
		return 0, false, nil
	}

	var out strings.Builder
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
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

// TestRun_ReattachedClose_NoPriorSessionInLog_OmitsMetadata verifies the
// ticket 06a edge case: when the run log has no iteration-started event to
// recover a session id from, the reattached close still marks the ticket
// done, without writing a wrong/placeholder actual_context_window.
func TestRun_ReattachedClose_NoPriorSessionInLog_OmitsMetadata(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-iter-01", Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}

	var out strings.Builder
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
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
