package prompt_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

const firstPromptGrace = servertest.FirstPromptGrace

// startFirstPromptSession starts a session with an input box and a first
// prompt, still waiting for its ready report.
func startFirstPromptSession(t *testing.T, prompt string, extraEnv ...string) (*servertest.Harness, context.Context, <-chan wire.Event, string, string) {
	t.Helper()
	return startFakeSessionWith(t, wire.CreateSessionRequest{Prompt: prompt}, func(c *server.Config) {
		c.PromptGrace = firstPromptGrace
		c.ActionSlashSettle = 200 * time.Millisecond
	}, servertest.WithEnv(append([]string{agentfake.ProgramInputEnv + "=1"}, extraEnv...)...))
}

// The agent's input box and the claude config dir come only from the server's
// Config.Env; the harness default HOME is poisoned with vim mode, so a read of
// HOME instead of CLAUDE_CONFIG_DIR would press `i` into the prompt.
func TestFirstPrompt_TypedWhenTheAgentEnvAndConfigDirComeFromServerEnv(t *testing.T) {
	t.Parallel()
	_, _, _, _, conv := startFakeSessionWith(t, wire.CreateSessionRequest{Prompt: "fix the bug"}, func(c *server.Config) {
		c.PromptGrace = firstPromptGrace
	}, servertest.WithEnv(agentfake.ProgramInputEnv+"=1"))
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
}

// toVimNormal is a mid-session /config toggle to vim followed by NORMAL: the
// launch had set normal editing, so only a toggle makes the probe run.
func toVimNormal(t *testing.T, conv string) {
	t.Helper()
	for _, action := range []string{agentfake.ActionVimToggle, agentfake.ActionVimNormal} {
		if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: action}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFirstPrompt_RealPromptInVimNormalIsDeliveredWithTheConfigDirFromServerEnv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, _, _, _, conv := startFakeSessionWith(t, wire.CreateSessionRequest{Prompt: "fix the bug"}, func(c *server.Config) {
		c.PromptGrace = firstPromptGrace
	}, servertest.WithEnv(agentfake.ProgramInputEnv+"=1", "CLAUDE_CONFIG_DIR="+dir))
	toVimNormal(t, conv)
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
	_, _, _, _, conv := startFirstPromptSession(t, "fix the bug", "CLAUDE_CONFIG_DIR="+dir)
	toVimNormal(t, conv)
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
}

// A vim INSERT box that draws no marker must not make the probe press `i`: the
// prompt arrives with no stray character.
func TestFirstPrompt_InsertWithoutMarkerGetsTheCleanPrompt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"editorMode":"vim"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, conv := startFirstPromptSession(t, "fix the bug", "CLAUDE_CONFIG_DIR="+dir, agentfake.ProgramInsertNoMarkerEnv+"=1")
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
}

// After a mid-session toggle to vim the probe runs, and its decision reaches
// server.log under the first prompt's caller label.
func TestFirstPrompt_ToggleToVimIsProbedAndLogged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	h, _, _, _, conv := startFirstPromptSession(t, "fix the bug", "CLAUDE_CONFIG_DIR="+dir)
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionVimToggle}); err != nil {
		t.Fatal(err)
	}
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
	logs, _ := os.ReadFile(server.LogPath(h.StateDir))
	for _, want := range []string{"vim probe", `"caller":"first-prompt"`} {
		if !strings.Contains(string(logs), want) {
			t.Errorf("server log lacks %q:\n%s", want, logs)
		}
	}
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
