package ralphloop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/herdr"
)

// TestRun_ReattachedSmartZoneBreach_AutoRecoversThenLands covers ticket 23's
// first requirement: a reattached iteration's wait still proactively observes
// context occupancy against --smart-zone and auto-recovers a breach (Ctrl-C,
// /compact, finish-up re-prompt), the same as a fresh iteration's wait —
// because reattachIteration drives its wait through the same waitForFinish
// call. Ends in an ordinary landing with the reattached worktree/tab/branch
// cleaned up exactly once.
func TestRun_ReattachedSmartZoneBreach_AutoRecoversThenLands(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	d, _, removed := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID, AgentStatus: "working"}}, nil
	}
	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		if strings.Contains(cwd, "epic-item-01") {
			return 999999, true, nil
		}
		return 0, false, nil
	}
	var waits int
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waits++
		if waits == 1 {
			return herdr.Agent{}, errors.New("timed out waiting for agent status")
		}
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}
	var sentKeys [][]string
	d.AgentSendKeys = func(target string, keys ...string) error {
		sentKeys = append(sentKeys, keys)
		return nil
	}
	var prompts []string
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		prompts = append(prompts, opts.Text)
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(sentKeys) != 1 || len(sentKeys[0]) == 0 || sentKeys[0][0] != "ctrl+c" {
		t.Errorf("AgentSendKeys calls = %v, want a single ctrl+c interrupt for the smart-zone breach", sentKeys)
	}
	if len(prompts) != 2 || prompts[0] != "/compact" || !strings.Contains(prompts[1], "context window") {
		t.Fatalf("prompts = %v, want [/compact, finish-up prompt]", prompts)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the reattached iteration cleaned up exactly once", *removed)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket after reattached smart-zone recovery = %s, want status: done", raw)
	}
}

// TestRun_ReattachedCodexQuota_StructuredRecoveryThenLands covers ticket 23's
// second requirement: a reattached Codex iteration's wait still detects a
// structured (session-file-backed) quota exhaustion and pauses/resumes the
// gate around the reset. Per ticket 04, a pane still blocked once that reset
// completes must park for a human instead of being re-prompted — herdr
// 0.8.2 hard-rejects a prompt into a blocked pane, and even without that
// guard "continue" isn't an answer to whatever dialog the pane is sitting
// on.
func TestRun_ReattachedCodexQuota_StructuredRecoveryThenLands(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	d, _, removed := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID, AgentStatus: "working"}}, nil
	}
	var waits int
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waits++
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "blocked"}, nil
	}
	d.AgentGet = func(target string) (herdr.Agent, error) {
		return herdr.Agent{PaneID: "pane-" + target, WorkspaceID: "ws1", TabID: "tab-" + target, AgentStatus: "blocked", AgentSession: "session-" + target}, nil
	}
	var quotaChecks int
	d.ReadCodexRateLimit = func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
		quotaChecks++
		if quotaChecks > 1 {
			return codexsession.RateLimit{}, false, nil
		}
		return codexsession.RateLimit{Quota: "primary", ResetAt: time.Now().Add(-time.Second)}, true, nil
	}
	var prompts []string
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		prompts = append(prompts, opts.Text)
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
	}
	// The pane stays blocked forever in this fake — there is no dialog for a
	// human to answer here, only a scripted proof that the park happened. The
	// park poll is the run's only path to noticing an external status change,
	// so it doubles as that "human" here: on the first poll it reads back the
	// parked ticket to prove it landed on needs-answer with the reset's
	// pane never prompted, then marks it done directly, the same way a person
	// resolving the dialog by hand would let the run settle.
	path := ticketPath(scratchDir, "epic", "01-a.md")
	var parkPolls int
	d.ParkTimer = func(dur time.Duration) <-chan time.Time {
		parkPolls++
		if parkPolls == 1 {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("ReadFile %s: %v", path, err)
			} else if !strings.Contains(string(raw), "needs-answer") {
				t.Errorf("ticket at first park poll = %s, want needs-answer", raw)
			}
			if err := SetStatus(path, "done"); err != nil {
				t.Errorf("SetStatus %s: %v", path, err)
			}
		}
		return readyTimer(dur)
	}

	if err := Run(RunOptions{EpicName: "epic", Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(prompts) != 0 {
		t.Errorf("prompts = %v, want none — a pane herdr reports blocked must never be prompted", prompts)
	}
	if len(*removed) != 0 {
		t.Errorf("removed worktree branches = %v, want none — the ticket was resolved directly, not through the ordinary landing path", *removed)
	}
	if parkPolls == 0 {
		t.Errorf("run never parked (no park poll), want it to park on the blocked-after-reset ticket")
	}
}

// TestRun_ReattachedCodexQuota_PaneTextFallbackRecoversThenLands covers the
// other half of ticket 23's second requirement: when structured session data
// can't identify the block, a reattached Codex iteration's wait still falls
// back to reading the pane's recent output to detect the quota message. Per
// ticket 04, a pane still blocked once that reset completes must park for a
// human instead of being re-prompted.
func TestRun_ReattachedCodexQuota_PaneTextFallbackRecoversThenLands(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	d, _, removed := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-epic-iter-01", Label: "epic-iter-01", WorkspaceID: workspaceID, AgentStatus: "working"}}, nil
	}
	var waits int
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waits++
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "blocked"}, nil
	}
	d.AgentGet = func(target string) (herdr.Agent, error) {
		return herdr.Agent{PaneID: "pane-" + target, WorkspaceID: "ws1", TabID: "tab-" + target, AgentStatus: "blocked", AgentSession: "session-" + target}, nil
	}
	d.ReadCodexRateLimit = func(cwd, sessionID string) (codexsession.RateLimit, bool, error) {
		return codexsession.RateLimit{}, false, nil
	}
	d.ReadPaneRecent = func(pane string) (string, error) {
		return "You've hit your usage limit. Try again in 2 hours 33 minutes 12 seconds.", nil
	}
	var prompts []string
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		prompts = append(prompts, opts.Text)
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
	}
	// Same rationale as the structured-recovery test above: the pane stays
	// blocked forever here, so the park poll doubles as the "human" that
	// notices the park and resolves it, after proving it landed correctly.
	path := ticketPath(scratchDir, "epic", "01-a.md")
	var parkPolls int
	d.ParkTimer = func(dur time.Duration) <-chan time.Time {
		parkPolls++
		if parkPolls == 1 {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("ReadFile %s: %v", path, err)
			} else if !strings.Contains(string(raw), "needs-answer") {
				t.Errorf("ticket at first park poll = %s, want needs-answer", raw)
			}
			if err := SetStatus(path, "done"); err != nil {
				t.Errorf("SetStatus %s: %v", path, err)
			}
		}
		return readyTimer(dur)
	}

	if err := Run(RunOptions{EpicName: "epic", Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(prompts) != 0 {
		t.Errorf("prompts = %v, want none — a pane herdr reports blocked must never be prompted", prompts)
	}
	if len(*removed) != 0 {
		t.Errorf("removed worktree branches = %v, want none — the ticket was resolved directly, not through the ordinary landing path", *removed)
	}
	if parkPolls == 0 {
		t.Errorf("run never parked (no park poll), want it to park on the blocked-after-reset ticket")
	}
}
