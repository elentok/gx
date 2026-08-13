package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/testutil/herdrctl"
)

var (
	buildAgentfakeOnce sync.Once
	agentfakeBinDir    string
	buildAgentfakeErr  error
)

// agentfakeBinary builds testutil/agentfake's cmd once per test run, as the
// literal binary name "claude", and returns the directory containing it —
// callers prepend this directory onto the workspace's PATH so `herdr agent
// start --kind claude` execs the fake instead of a real Claude Code.
func agentfakeBinary(t *testing.T) string {
	t.Helper()
	buildAgentfakeOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gx-e2e-agentfake")
		if err != nil {
			buildAgentfakeErr = err
			return
		}
		bin := filepath.Join(dir, "claude")
		cmd := exec.Command("go", "build", "-o", bin, "github.com/elentok/gx/testutil/agentfake/cmd/agentfake")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildAgentfakeErr = err
			t.Logf("go build output:\n%s", out)
			return
		}
		agentfakeBinDir = dir
	})
	if buildAgentfakeErr != nil {
		t.Fatalf("herdrctl e2e: build agentfake binary: %v", buildAgentfakeErr)
	}
	return agentfakeBinDir
}

// TestCompactionFalseIdle_AgentWaitDoesNotSettleBeforeFakeCompactionEnds
// regression-tests the highest-churn documented herdr bug (the ~30-ticket
// compact-issue epic): herdr reporting an agent's turn as settled before a
// real `/compact`-style pause has actually finished. A fake "claude" (see
// testutil/agentfake) goes working for a fixed duration after receiving a
// prompt, then reports done; `herdr agent prompt --wait` (which settles on
// idle, done, or blocked) must not return before that duration elapses, and
// must report the turn as "done" (a completed turn), not "idle" (never
// started).
func TestCompactionFalseIdle_AgentWaitDoesNotSettleBeforeFakeCompactionEnds(t *testing.T) {
	herdrctl.RequireHerdr(t)

	const compactionPause = 6 * time.Second

	fakeDir := agentfakeBinary(t)
	repoDir := t.TempDir()

	ws := herdrctl.NewWorkspace(t, repoDir)
	ws.PrependPath(fakeDir)

	ws.AgentStart(herdr.AgentStartOptions{
		Name: "compaction-false-idle",
		Kind: "claude",
		AgentArgs: []string{
			"--mode=compact",
			"--duration=" + compactionPause.String(),
		},
	})

	start := time.Now()
	agent := ws.AgentPrompt(herdr.AgentPromptOptions{
		Text: "/compact",
		Wait: true,
	})
	elapsed := time.Since(start)

	if elapsed < compactionPause {
		t.Fatalf("agent prompt --wait returned after %s, before the fake compaction's %s pause elapsed — herdr reported the turn settled too early",
			elapsed, compactionPause)
	}
	if agent.AgentStatus != "done" {
		explain := ws.AgentExplain("")
		t.Fatalf("agent status = %q, want done (explain: state=%q rule=%q)", agent.AgentStatus, explain.State, explain.MatchedRuleID)
	}
}
