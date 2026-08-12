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
// structured (session-file-backed) quota exhaustion, pauses and resumes the
// gate around the reset, then re-prompts and lands normally — the same
// recovery waitForFinish performs for a fresh iteration.
func TestRun_ReattachedCodexQuota_StructuredRecoveryThenLands(t *testing.T) {
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
		if waits < 3 {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "blocked"}, nil
		}
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
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

	if err := Run(RunOptions{EpicName: "epic", Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(prompts) != 1 || prompts[0] != "continue" {
		t.Errorf("prompts = %v, want a single \"continue\" re-prompt after the structured Codex quota reset", prompts)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the reattached iteration cleaned up exactly once", *removed)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket after reattached structured quota recovery = %s, want status: done", raw)
	}
}

// TestRun_ReattachedCodexQuota_PaneTextFallbackRecoversThenLands covers the
// other half of ticket 23's second requirement: when structured session data
// can't identify the block, a reattached Codex iteration's wait still falls
// back to reading the pane's recent output to detect the quota message, and
// recovers the same way.
func TestRun_ReattachedCodexQuota_PaneTextFallbackRecoversThenLands(t *testing.T) {
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
		if waits < 3 {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "blocked"}, nil
		}
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
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

	if err := Run(RunOptions{EpicName: "epic", Agent: AgentCodex, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(prompts) != 1 || prompts[0] != "continue" {
		t.Errorf("prompts = %v, want a single \"continue\" re-prompt after the pane-text Codex quota reset", prompts)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the reattached iteration cleaned up exactly once", *removed)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket after reattached pane-text quota recovery = %s, want status: done", raw)
	}
}
