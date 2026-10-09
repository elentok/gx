package nativerunner_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

func blockedHeadless(t *testing.T) (*nativerunner.Headless, agentrunner.Session, string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(agentfake.ClaudeEnv, "1")
	root := t.TempDir()
	t.Cleanup(func() { killAgents(root) })
	h := &nativerunner.Headless{Root: root, Claude: exe, PollInterval: 5 * time.Millisecond, PromptTimeout: time.Second}
	s, err := h.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Prompt(s, "do it"); err != nil {
		t.Fatal(err)
	}
	if err := agentfake.SendControl(filepath.Join(root, s.Label, nativerunner.StdinFile), agentfake.Control{Action: agentfake.ActionBlock}); err != nil {
		t.Fatal(err)
	}
	return h, s, root
}

func TestHeadless_BlockedReasonNamesToolAndInput(t *testing.T) {
	h, s, _ := blockedHeadless(t)
	st, err := h.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.BlockedReason, "Bash") || !strings.Contains(st.BlockedReason, `"command":"ls"`) {
		t.Fatalf("BlockedReason = %q, want tool and input", st.BlockedReason)
	}
}

func TestHeadless_DenyTellsClaudeWhy(t *testing.T) {
	h, s, root := blockedHeadless(t)
	if _, err := h.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := h.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionDeny, Text: "not now"}); err != nil {
		t.Fatal(err)
	}
	// A second answer has nothing left to answer.
	if err := h.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow}); err == nil {
		t.Fatal("second Answer succeeded, want ErrNotReady")
	}
	waitForOut(t, filepath.Join(root, s.Label, nativerunner.OutFile), "permission deny not now")
}

func TestHeadless_PendingRequestAnswerableAfterRestart(t *testing.T) {
	h, s, root := blockedHeadless(t)
	if _, err := h.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	r := &nativerunner.Headless{Root: root, Claude: h.Claude, PollInterval: 5 * time.Millisecond}
	s2, _, err := r.Reattach(s.Label)
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Wait(s2, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.BlockedReason, "Bash") {
		t.Fatalf("BlockedReason after restart = %q, want the tool", st.BlockedReason)
	}
	if err := r.Answer(s2, agentrunner.Answer{Decision: agentrunner.DecisionAllow}); err != nil {
		t.Fatal(err)
	}
	waitForOut(t, filepath.Join(root, s.Label, nativerunner.OutFile), "permission allow")
}

func waitForOut(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, _ := os.ReadFile(path); strings.Contains(string(data), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never contained %q", path, want)
}
