package harness_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/harness"
)

func claude(t *testing.T) harness.Harness {
	t.Helper()
	h, err := harness.Lookup(harness.Claude)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestLookup_UnknownNameIsAnError(t *testing.T) {
	if _, err := harness.Lookup("nope"); err == nil {
		t.Fatal("Lookup(nope) succeeded")
	}
}

func TestLookup_FindsARegisteredHarness(t *testing.T) {
	if _, err := harness.Lookup("registered"); err == nil {
		t.Fatal("Lookup(registered) succeeded before Register")
	}
	harness.Register("registered", claude(t))
	if _, err := harness.Lookup("registered"); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeLaunch(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	cmd := claude(t).Launch(harness.LaunchOptions{SessionID: "s1", Args: []string{"--x"}, Env: []string{"FOO=bar"}})
	if cmd.Name != "claude" || !slices.Equal(cmd.Args, []string{"--session-id", "s1", "--x"}) {
		t.Fatalf("Launch = %s %q", cmd.Name, cmd.Args)
	}
	assertClaudeEnv(t, cmd.Env)
}

func TestClaudeResume(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	cmd, ok := claude(t).Resume("conv-1", harness.LaunchOptions{Args: []string{"--x"}, Env: []string{"FOO=bar"}})
	if !ok || cmd.Name != "claude" || !slices.Equal(cmd.Args, []string{"--resume", "conv-1", "--x"}) {
		t.Fatalf("Resume = %s %q, ok=%v", cmd.Name, cmd.Args, ok)
	}
	assertClaudeEnv(t, cmd.Env)
}

func assertClaudeEnv(t *testing.T, env []string) {
	t.Helper()
	if slices.Contains(env, "CLAUDECODE=1") || !slices.Contains(env, "DISABLE_AUTOUPDATER=1") || !slices.Contains(env, "FOO=bar") {
		t.Fatalf("env not cleaned up: %q", env)
	}
}

func TestClaudeWatch_ReadsPidFileUnderConfigDir(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	sessions := filepath.Join(cfg, "sessions")
	if err := os.Mkdir(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	defer close(stop)
	spawned := time.Now()
	w, err := claude(t).Watch(4242, spawned, stop)
	if err != nil {
		t.Fatal(err)
	}
	if got := w.Status(); got.State != harness.Starting {
		t.Fatalf("Status = %+v, want Starting", got)
	}
	data := fmt.Sprintf(`{"pid":4242,"sessionId":"conv-1","startedAt":%d,"status":"busy"}`, spawned.UnixMilli())
	if err := os.WriteFile(filepath.Join(sessions, "4242.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for w.Status().State != harness.Working || w.ConversationID() != "conv-1" {
		if time.Now().After(deadline) {
			t.Fatalf("Status = %+v, ConversationID = %q", w.Status(), w.ConversationID())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestClaudeStartupScreen(t *testing.T) {
	rows := []string{
		"Quick safety check: Is this a project you created or one you trust?",
		"❯ 1. No, exit",
		"  2. Yes, I trust this folder",
	}
	if got := claude(t).StartupScreen(nil, rows); got != (harness.Startup{Dialog: harness.TrustDialog, Key: "\x1b[B"}) {
		t.Fatalf("trust dialog: %+v", got)
	}
	if got := claude(t).StartupScreen(nil, []string{"loading"}); got.Dialog != harness.NoDialog {
		t.Fatalf("no dialog: %+v", got)
	}
}

func TestClaudeDisplayName(t *testing.T) {
	if got, ok := claude(t).DisplayName([]byte("x\x1b]0;✳ Fix bug\x07y")); !ok || got != "✳ Fix bug" {
		t.Fatalf("DisplayName = %q, %v", got, ok)
	}
}
