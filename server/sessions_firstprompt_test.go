package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

const firstPromptGrace = 300 * time.Millisecond

// startFirstPromptSession starts a session with an input box and a first
// prompt, still waiting for its ready report.
func startFirstPromptSession(t *testing.T, prompt string, extraEnv ...string) (*servertest.Harness, context.Context, <-chan server.Event, string, string) {
	t.Helper()
	return startFakeSessionWith(t, server.CreateSessionRequest{Prompt: prompt}, func(c *server.Config) {
		c.PromptGrace = firstPromptGrace
		c.ActionSlashSettle = 200 * time.Millisecond
	}, servertest.WithEnv(append([]string{agentfake.ProgramInputEnv + "=1"}, extraEnv...)...))
}

// The agent's input box and the claude config dir come only from the server's
// Config.Env; the harness default HOME is poisoned with vim mode, so a read of
// HOME instead of CLAUDE_CONFIG_DIR would press `i` into the prompt.
func TestFirstPrompt_TypedWhenTheAgentEnvAndConfigDirComeFromServerEnv(t *testing.T) {
	t.Parallel()
	_, _, _, _, conv := startFakeSessionWith(t, server.CreateSessionRequest{Prompt: "fix the bug"}, func(c *server.Config) {
		c.PromptGrace = firstPromptGrace
	}, servertest.WithEnv(agentfake.ProgramInputEnv+"=1"))
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
}

func TestFirstPrompt_RealPromptInVimNormalIsDeliveredWithTheConfigDirFromServerEnv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"editorMode":"vim"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, conv := startFakeSessionWith(t, server.CreateSessionRequest{Prompt: "fix the bug"}, func(c *server.Config) {
		c.PromptGrace = firstPromptGrace
	}, servertest.WithEnv(agentfake.ProgramInputEnv+"=1", "CLAUDE_CONFIG_DIR="+dir))
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionVimNormal}); err != nil {
		t.Fatal(err)
	}
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
}

func TestFirstPrompt_TypedOnReadyReportAndSubmittedOnce(t *testing.T) {
	t.Parallel()
	_, _, _, _, conv := startFirstPromptSession(t, "fix the bug")
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
	report(t, conv, agentfake.Control{State: "working"})
	time.Sleep(2 * firstPromptGrace) // negative wait: a repeat would come within the grace window
	if got := submitted(t, conv); got != "\"fix the bug\"\n" {
		t.Errorf("submitted = %q, want exactly one submission", got)
	}
}

// The draft check must read the screen after ready-to-type switched vim to
// INSERT, not while the box still sits in NORMAL.
func TestFirstPrompt_RealPromptInVimNormalIsDeliveredAfterTheInsertSwitch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"editorMode":"vim"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, conv := startFirstPromptSession(t, "fix the bug", "CLAUDE_CONFIG_DIR="+dir)
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionVimNormal}); err != nil {
		t.Fatal(err)
	}
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
}

func TestFirstPrompt_RealPromptWithSwallowedEnterIsNudged(t *testing.T) {
	t.Parallel()
	_, _, _, _, conv := startFirstPromptSession(t, "fix the bug")
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
		t.Fatal(err)
	}
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
}

func TestFirstPrompt_SlashPromptIsTypedOnceAndGetsOneEnter(t *testing.T) {
	t.Parallel()
	_, _, _, _, conv := startFirstPromptSession(t, "/local")
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
		t.Fatal(err)
	}
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"/local\"\n")
	time.Sleep(3 * firstPromptGrace) // negative wait: a second Enter would come within a few grace windows
	if got := submitted(t, conv); got != "\"/local\"\n" {
		t.Errorf("submitted = %q, want one submission", got)
	}
}

func TestFirstPrompt_SlashPromptKeepsPressingEnterWhileItSitsUnsent(t *testing.T) {
	t.Parallel()
	_, _, _, _, conv := startFirstPromptSession(t, "/local")
	for range 2 {
		if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
			t.Fatal(err)
		}
	}
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"/local\"\n")
}

func TestFirstPrompt_PersonInputAfterTypingStopsTheNudge(t *testing.T) {
	t.Parallel()
	for name, prompt := range map[string]string{"real": "fix the bug", "slash": "/local"} {
		t.Run(name, func(t *testing.T) {
			h, _, _, id, conv := startFirstPromptSession(t, prompt)
			if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
				t.Fatal(err)
			}
			holder := waitForHolder(t, filepath.Join(h.SessionsDir, id))
			report(t, conv, agentfake.Control{State: "idle"})
			waitForScreen(t, holder, prompt)

			a, err := holder.Attach()
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if err := a.Paste(" "); err != nil {
				t.Fatal(err)
			}
			time.Sleep(3 * firstPromptGrace) // negative wait: a nudge would come within a few grace windows
			if got := submitted(t, conv); got != "" {
				t.Errorf("submitted = %q after person input, want nothing", got)
			}
		})
	}
}
