package promptflows_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// tuiChat is a chat session on the TUI fake, Idle, with no first prompt.
type tuiChat struct {
	h          *servertest.Harness
	ctx        context.Context
	evs        <-chan wire.Event
	id, conv   string
	home, path string
}

// startTUIChat starts the session with env (KEY=VALUE) set for the fake. The
// slash settle is short so a swallowed Enter is pressed again within a test.
func startTUIChat(t *testing.T, env ...string) tuiChat {
	t.Helper()
	return startTUIChatPrompt(t, "", env...)
}

// startTUIChatPrompt starts the chat with a first prompt; with none it waits for Idle.
func startTUIChatPrompt(t *testing.T, prompt string, env ...string) tuiChat {
	t.Helper()
	bin, home := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\nexport " + agentfake.TUIEnv + "=1 " + strings.Join(env, " ") + "\nexec '" + os.Args[0] + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder(),
		func(c *server.Config) {
			c.ActionSlashSettle = 200 * time.Millisecond
			c.PromptGrace = firstPromptGrace
			c.ActionStepStartTimeout = firstPromptGrace
			if prompt != "" {
				c.LaunchProofCaps.Grace = 4 * time.Second // the proof of an undelivered first prompt ends soon
			}
		},
		servertest.WithEnv("PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "HOME="+home, "CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude")))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: dir, Harness: "claude", Prompt: prompt})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := servertest.ReadMeta(t, h, id)["conversation_id"].(string)
	if prompt == "" {
		servertest.WaitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	}
	return tuiChat{h: h, ctx: ctx, evs: evs, id: id, conv: conv, home: home}
}

func (c tuiChat) send(t *testing.T, text string) {
	t.Helper()
	if res, err := c.h.Client.Sessions.Prompt(c.ctx, wire.SessionPromptRequest{ID: c.id, Text: text}); err != nil || res.Refused {
		t.Fatalf("prompt %q = %+v, %v", text, res, err)
	}
}

// keys is every key the fake read off its terminal.
func (c tuiChat) keys() string {
	data, _ := os.ReadFile(agentfake.TUIInputPath(c.home, c.conv))
	return string(data)
}

func (c tuiChat) waitKeys(t *testing.T, what string, ok func(keys string) bool) {
	t.Helper()
	servertest.WaitFor(t, 20*time.Second, what, func() bool { return ok(c.keys()) })
}

// TestSessionPromptSlash holds the chat send flows on the TUI fake.
func TestSessionPromptSlash(t *testing.T) {
	t.Parallel()
	t.Run("HiddenCharacterSwallowsTheFirstEnter", func(t *testing.T) {
		t.Parallel()
		c := startTUIChat(t)
		const text = "/co\U0001BCA0st"
		c.send(t, text)
		c.waitKeys(t, "two Enters", func(k string) bool { return strings.Count(k, "\r") >= 2 })
		time.Sleep(2 * firstPromptGrace) // negative wait: a third Enter or a retype would come within it
		keys := c.keys()
		if got := strings.Count(keys, text); got != 1 {
			t.Errorf("typed %d times, want once: %q", got, keys)
		}
		if got := strings.Count(keys, "\r"); got != 2 {
			t.Errorf("read %d Enters, want 2: %q", got, keys)
		}
	})
	t.Run("UnknownCommandFailsAtOnce", func(t *testing.T) {
		t.Parallel()
		c := startTUIChat(t, agentfake.TUIUnknownSkillEnv+"=1")
		c.send(t, "/my-skill")
		got := servertest.WaitForSession(c.ctx, t, c.h, c.evs, c.id, func(s wire.SessionInfo) bool { return s.Actionable.Reason != "" }).Actionable
		if !strings.HasPrefix(got.Reason, "Send failed: ") || !strings.Contains(got.Reason, "Unknown command") || len(got.Action) != 1 || got.Action[0] != "/my-skill" {
			t.Errorf("card = %+v, want the red Send failed card with the text to resend", got)
		}
		if n := strings.Count(c.keys(), "/my-skill"); n != 1 {
			t.Errorf("typed %d times, want once: %q", n, c.keys())
		}
	})
	t.Run("CompactIsNudgedAndRunsOnce", func(t *testing.T) {
		t.Parallel()
		c := startTUIChat(t)
		if err := agentfake.SendTUIControl(c.home, c.conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
			t.Fatal(err)
		}
		c.send(t, "/compact")
		servertest.WaitFor(t, 20*time.Second, "the compaction", func() bool {
			return len(agentfake.ReadTUILocalCommands(c.home, c.conv)) > 0
		})
		time.Sleep(2 * firstPromptGrace) // negative wait: a second run would come within it
		if got := agentfake.ReadTUILocalCommands(c.home, c.conv); len(got) != 1 || got[0] != "compact-start" {
			t.Errorf("local commands = %v, want one compact-start", got)
		}
	})
	t.Run("DraftIsClearedBeforePlainText", func(t *testing.T) {
		t.Parallel()
		c := startTUIChat(t, agentfake.TUIDraftEnv+"=old_draft")
		c.send(t, "hello")
		c.waitKeys(t, "the text typed", func(k string) bool { return strings.Contains(k, "hello") })
		keys := c.keys()
		if clear := strings.Index(keys, "\x15"); clear < 0 || clear > strings.Index(keys, "hello") {
			t.Errorf("keys = %q, want the clear key before the text", keys)
		}
	})
	t.Run("PlainTextIsTypedWithoutAZeroWidthSpace", func(t *testing.T) {
		t.Parallel()
		c := startTUIChat(t)
		c.send(t, "he​llo")
		c.waitKeys(t, "the text typed", func(k string) bool { return strings.Contains(k, "hello") })
		if keys := c.keys(); strings.Contains(keys, "​") {
			t.Errorf("keys = %q, want no zero-width space", keys)
		}
	})
}
