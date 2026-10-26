package harness_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
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
	if cmd.Name != "claude" || !slices.Equal(cmd.Args, []string{"--session-id", "s1", "--permission-mode", "auto", "--x"}) {
		t.Fatalf("Launch = %s %q", cmd.Name, cmd.Args)
	}
	assertClaudeEnv(t, cmd.Env)
}

func TestClaudeResume(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	cmd, ok := claude(t).Resume("conv-1", harness.LaunchOptions{Args: []string{"--x"}, Env: []string{"FOO=bar"}})
	if !ok || cmd.Name != "claude" || !slices.Equal(cmd.Args, []string{"--resume", "conv-1", "--permission-mode", "auto", "--x"}) {
		t.Fatalf("Resume = %s %q, ok=%v", cmd.Name, cmd.Args, ok)
	}
	assertClaudeEnv(t, cmd.Env)
}

func TestClaudeLaunchKeepsCallerPermissionMode(t *testing.T) {
	cmd := claude(t).Launch(harness.LaunchOptions{SessionID: "s1", Args: []string{"--permission-mode", "auto", "--x"}})
	if !slices.Equal(cmd.Args, []string{"--session-id", "s1", "--permission-mode", "auto", "--x"}) {
		t.Fatalf("Launch = %q", cmd.Args)
	}
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
	w, err := claude(t).Watch(4242, spawned, rowsScreen{"loading"}, stop)
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

// rowsScreen is a screen with nothing scrolled off.
type rowsScreen []string

func (s rowsScreen) Rows() []string              { return s }
func (s rowsScreen) ScrollbackTail(int) []string { return nil }

const claudeuiTestdata = "../agentrunner/claudeui/testdata"

func TestClaudeWatch_PreSessionDialogAtAnyWidth(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(claudeuiTestdata, "trust-w60.raw"))
	if err != nil {
		t.Fatal(err)
	}
	narrow := screen.NewScreen(60, 30)
	t.Cleanup(func() { _ = narrow.Close() })
	if _, err := narrow.Write(raw); err != nil {
		t.Fatal(err)
	}
	wide, err := os.ReadFile(filepath.Join(claudeuiTestdata, "trust-w160.screen.txt"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		scr  harness.Screen
		want harness.Status
	}{
		{"trust dialog at 60 cols", narrow, harness.Status{State: harness.Blocked, Blocked: harness.BlockedDialog}},
		{"trust dialog at 160 cols", rowsScreen(strings.Split(string(wide), "\n")), harness.Status{State: harness.Blocked, Blocked: harness.BlockedDialog}},
		{"no dialog", rowsScreen{"loading"}, harness.Status{State: harness.Starting}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
			stop := make(chan struct{})
			defer close(stop)
			w, err := claude(t).Watch(4242, time.Now(), c.scr, stop)
			if err != nil {
				t.Fatal(err)
			}
			if got := w.Status(); got != c.want {
				t.Fatalf("Status = %+v, want %+v", got, c.want)
			}
		})
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

// TestClaudeDisplayName: claude prefixes its title with a status glyph (✳
// idle, a braille spinner while working); the name drops it, as the Agents
// tab already shows the state.
func TestClaudeDisplayName(t *testing.T) {
	for _, title := range []string{"✳ Fix bug", "⠂ Fix bug", "⠐ Fix bug", "Fix bug"} {
		if got, ok := claude(t).DisplayName([]byte("x\x1b]0;" + title + "\x07y")); !ok || got != "Fix bug" {
			t.Errorf("DisplayName(%q) = %q, %v", title, got, ok)
		}
	}
	if got, ok := claude(t).DisplayName([]byte("\x1b]0;✳ \x07")); ok {
		t.Errorf("glyph-only title = %q, want none", got)
	}
}
