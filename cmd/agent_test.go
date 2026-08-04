package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func agentTestDeps(stdout *bytes.Buffer, env map[string]string) deps {
	return deps{
		stdout: stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return "/repo", nil },
		getenv: func(key string) string { return env[key] },
		readClaudeOccupancy: func(cwd, sessionID string) (int, bool, error) {
			return 0, false, errors.New("readClaudeOccupancy not stubbed")
		},
		verifyCodexSession: func(cwd, sessionID string) (bool, error) {
			return false, errors.New("verifyCodexSession not stubbed")
		},
		readCodexContext: func(cwd, sessionID string) (int, bool, error) {
			return 0, false, errors.New("readCodexContext not stubbed")
		},
	}
}

func TestExecute_AgentContextWindow_ClaudeSessionDetected(t *testing.T) {
	var stdout bytes.Buffer
	d := agentTestDeps(&stdout, map[string]string{"CLAUDE_CODE_SESSION_ID": "sess-1"})
	d.readClaudeOccupancy = func(cwd, sessionID string) (int, bool, error) {
		if cwd != "/repo" || sessionID != "sess-1" {
			t.Errorf("readClaudeOccupancy(%q, %q), want (/repo, sess-1)", cwd, sessionID)
		}
		return 12345, true, nil
	}

	if err := execute([]string{"agent", "context-window"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "12345" {
		t.Errorf("stdout = %q, want %q", got, "12345")
	}
}

func TestExecute_AgentContextWindow_CodexSessionDetected(t *testing.T) {
	var stdout bytes.Buffer
	d := agentTestDeps(&stdout, map[string]string{"CODEX_THREAD_ID": "thread-1"})
	d.verifyCodexSession = func(cwd, sessionID string) (bool, error) {
		return cwd == "/repo" && sessionID == "thread-1", nil
	}
	d.readCodexContext = func(cwd, sessionID string) (int, bool, error) {
		return 999, true, nil
	}

	if err := execute([]string{"agent", "context-window"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "999" {
		t.Errorf("stdout = %q, want %q", got, "999")
	}
}

func TestExecute_AgentContextWindow_NeitherProviderFails(t *testing.T) {
	var stdout bytes.Buffer
	d := agentTestDeps(&stdout, map[string]string{})

	err := execute([]string{"agent", "context-window"}, d)
	if err == nil {
		t.Fatal("expected error when no session env var is set, got nil")
	}
	if !strings.Contains(err.Error(), "no agent session detected") {
		t.Errorf("error = %q, want it to mention no session detected", err.Error())
	}
}

func TestExecute_AgentContextWindow_BothProvidersAmbiguous(t *testing.T) {
	var stdout bytes.Buffer
	d := agentTestDeps(&stdout, map[string]string{
		"CLAUDE_CODE_SESSION_ID": "sess-1",
		"CODEX_THREAD_ID":        "thread-1",
	})

	err := execute([]string{"agent", "context-window"}, d)
	if err == nil {
		t.Fatal("expected error when both provider env vars are set, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want it to mention ambiguity", err.Error())
	}
}

func TestExecute_AgentContextWindow_IdentityMismatchFails(t *testing.T) {
	var stdout bytes.Buffer
	d := agentTestDeps(&stdout, map[string]string{"CLAUDE_CODE_SESSION_ID": "sess-1"})
	d.readClaudeOccupancy = func(cwd, sessionID string) (int, bool, error) {
		return 0, false, nil
	}

	err := execute([]string{"agent", "context-window"}, d)
	if err == nil {
		t.Fatal("expected error when the session's transcript can't be found, got nil")
	}
	if !strings.Contains(err.Error(), "no claude transcript found") {
		t.Errorf("error = %q, want it to mention the missing transcript", err.Error())
	}
}

func TestExecute_AgentContextWindow_CodexIdentityMismatchFails(t *testing.T) {
	var stdout bytes.Buffer
	d := agentTestDeps(&stdout, map[string]string{"CODEX_THREAD_ID": "thread-1"})
	d.verifyCodexSession = func(cwd, sessionID string) (bool, error) {
		return false, nil
	}

	err := execute([]string{"agent", "context-window"}, d)
	if err == nil {
		t.Fatal("expected error when codex identity doesn't verify, got nil")
	}
	if !strings.Contains(err.Error(), "no codex session") {
		t.Errorf("error = %q, want it to mention the unverified session", err.Error())
	}
}

func TestExecute_AgentContextWindow_ExplicitOverridesSkipEnv(t *testing.T) {
	var stdout bytes.Buffer
	// Env vars point at a different session than the explicit override, to
	// prove the override wins rather than falling back to auto-detection.
	d := agentTestDeps(&stdout, map[string]string{"CLAUDE_CODE_SESSION_ID": "env-session"})
	d.readClaudeOccupancy = func(cwd, sessionID string) (int, bool, error) {
		if cwd != "/other" || sessionID != "explicit-session" {
			t.Errorf("readClaudeOccupancy(%q, %q), want (/other, explicit-session)", cwd, sessionID)
		}
		return 42, true, nil
	}

	err := execute([]string{
		"agent", "context-window",
		"--agent", "claude",
		"--session", "explicit-session",
		"--cwd", "/other",
	}, d)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "42" {
		t.Errorf("stdout = %q, want %q", got, "42")
	}
}

func TestExecute_AgentContextWindow_SessionWithoutAgentFails(t *testing.T) {
	var stdout bytes.Buffer
	d := agentTestDeps(&stdout, map[string]string{})

	err := execute([]string{"agent", "context-window", "--session", "sess-1"}, d)
	if err == nil {
		t.Fatal("expected error for --session without --agent, got nil")
	}
	if !strings.Contains(err.Error(), "--session requires --agent") {
		t.Errorf("error = %q, want it to mention --session requires --agent", err.Error())
	}
}

func TestExecute_AgentContextWindow_InvalidAgentFails(t *testing.T) {
	var stdout bytes.Buffer
	d := agentTestDeps(&stdout, map[string]string{})

	err := execute([]string{"agent", "context-window", "--agent", "bogus"}, d)
	if err == nil {
		t.Fatal("expected error for an unknown --agent value, got nil")
	}
	if !strings.Contains(err.Error(), `invalid --agent`) {
		t.Errorf("error = %q, want it to mention the invalid --agent value", err.Error())
	}
}
