package prompt_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/transcript"
)

const launchPrompt = "/gx-implement gx:epic/01"

type launchFixture struct {
	firstPromptFixture
	home string
	dir  string
}

// startLaunchSession starts a claude session whose "claude" is the TUI fake
// with env, and a slash first prompt.
func startLaunchSession(t *testing.T, env ...string) launchFixture {
	t.Helper()
	return startLaunchSessionWith(t, nil, env...)
}

func startLaunchSessionWith(t *testing.T, tune func(*server.Config), env ...string) launchFixture {
	t.Helper()
	bin, home := t.TempDir(), t.TempDir()
	// The fake writes its transcript under CLAUDE_CONFIG_DIR, where transcript.PathIn looks.
	config := filepath.Join(home, ".claude")
	exports := "export " + agentfake.TUIEnv + "=1"
	for _, e := range env {
		exports += " '" + e + "'"
	}
	script := "#!/bin/sh\n" + exports + "\nexec '" + os.Args[0] + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, t.TempDir(),
		servertest.WithTestHolder(),
		func(c *server.Config) {
			c.LaunchProofCaps.Grace = 6 * time.Second
			c.LaunchProofCaps.UnknownGrace = 600 * time.Millisecond
			c.LaunchProofCaps.Poll = 20 * time.Millisecond
			c.LaunchProofCaps.ResendEvery = 200 * time.Millisecond
			if tune != nil {
				tune(c)
			}
		},
		servertest.WithEnv("PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "HOME="+home, "CLAUDE_CONFIG_DIR="+config),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: dir, Harness: "claude", Prompt: launchPrompt})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := readMeta(t, h, id)["conversation_id"].(string)
	return launchFixture{firstPromptFixture{h: h, ctx: ctx, evs: evs, id: id, conv: conv}, home, dir}
}

func (f launchFixture) transcript() string {
	data, _ := os.ReadFile(transcript.PathIn(f.home, f.dir, f.conv))
	return string(data)
}

// launchRecords counts the transcript's person messages that are the launch command.
func (f launchFixture) launchRecords() int {
	msgs, _, _ := transcript.ReadChat(transcript.PathIn(f.home, f.dir, f.conv), "")
	n := 0
	for _, m := range msgs {
		if m.Role == transcript.ChatRolePerson && m.Text == launchPrompt {
			n++
		}
	}
	return n
}

func (f launchFixture) waitForUserRecord(t *testing.T) {
	t.Helper()
	servertest.WaitFor(t, 15*time.Second, "the launch command in the transcript", func() bool { return f.launchRecords() > 0 })
}

func (f launchFixture) assertNoActionable(t *testing.T) {
	t.Helper()
	snap, err := f.h.Client.Snapshot(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Sessions {
		if s.ID == f.id && s.Actionable.GxRaised {
			t.Errorf("actionable = %+v, want none", s.Actionable)
		}
	}
}

func TestFirstPromptLaunch_AlwaysUnknownSlashPromptEndsInAVisibleFailure(t *testing.T) {
	t.Parallel()
	f := startLaunchSession(t, agentfake.TUIUnknownSkillEnv+"=1")
	got := f.gxActionable(t)
	if !strings.Contains(got.Reason, "Unknown command:") || len(got.Action) != 1 || got.Action[0] != launchPrompt {
		t.Errorf("actionable = %+v, want the prompt as a one-step Action and an Unknown command reason", got)
	}
}

func TestFirstPromptLaunch_LateRegistryIsDeliveredAfterResends(t *testing.T) {
	t.Parallel()
	// The registry fills after the unknown grace would have ended in the
	// always-unknown test, so only a longer grace lets the resends win.
	f := startLaunchSessionWith(t, func(c *server.Config) { c.LaunchProofCaps.UnknownGrace = 5 * time.Second },
		agentfake.TUIRegistryDelayEnv+"=1500ms")
	f.waitForUserRecord(t)
	if n := strings.Count(f.transcript(), agentfake.UnknownCommandText("/gx-implement")); n < 2 {
		t.Errorf("unknown-command records = %d, want the prompt resent at least once", n)
	}
	f.assertNoActionable(t)
}

func TestFirstPromptLaunch_AcceptedSlashPromptIsSentOnce(t *testing.T) {
	t.Parallel()
	f := startLaunchSession(t)
	f.waitForUserRecord(t)
	time.Sleep(time.Second) // negative wait: a resend would come within a few ResendEvery
	if n := f.launchRecords(); n != 1 {
		t.Errorf("launch records = %d, want exactly 1", n)
	}
	f.assertNoActionable(t)
}

const vimEvent = "shown after an editorMode normal launch"

func (f launchFixture) vimEvents() int {
	data, _ := os.ReadFile(server.LogPath(f.h.StateDir))
	return strings.Count(string(data), vimEvent)
}

// typedIn counts the launch command records in conversation conv's transcript.
func (f launchFixture) typedIn(conv string) int {
	msgs, _, _ := transcript.ReadChat(transcript.PathIn(f.home, f.dir, conv), "")
	n := 0
	for _, m := range msgs {
		if m.Text == launchPrompt {
			n++
		}
	}
	return n
}

func (f launchFixture) currentConv(t *testing.T) string {
	t.Helper()
	conv, _ := readMeta(t, f.h, f.id)["conversation_id"].(string)
	return conv
}

func wedge(t *testing.T, mode string, launches int) string {
	return agentfake.TUIVimWedgeEnv + "=" + mode + ":" + strconv.Itoa(launches) + ":" + filepath.Join(t.TempDir(), "launches")
}

func TestFirstPromptVim(t *testing.T) {
	t.Parallel()
	t.Run("FooterOnceRestartsOnceAndTypesInTheNewConversation", func(t *testing.T) {
		t.Parallel()
		for _, mode := range []string{"INSERT", "NORMAL"} {
			t.Run(mode, func(t *testing.T) {
				t.Parallel()
				f := startLaunchSession(t, wedge(t, mode, 1))
				first := f.conv
				servertest.WaitFor(t, 30*time.Second, "the prompt typed in the new conversation", func() bool {
					conv := f.currentConv(t)
					return conv != first && f.typedIn(conv) > 0
				})
				if n := f.typedIn(first); n != 0 {
					t.Errorf("prompt typed in the first conversation %d times, want 0", n)
				}
				meta := readMeta(t, f.h, f.id)
				if meta["id"] != f.id || meta["run"].(float64) < 2 {
					t.Errorf("meta = %v, want the same id and a new run", meta)
				}
				if n := f.vimEvents(); n != 1 {
					t.Errorf("vim events = %d, want 1", n)
				}
				f.assertNoActionable(t)
			})
		}
	})

	t.Run("FooterOnBothLaunchesLogsTwiceAndEndsInTheExistingActionable", func(t *testing.T) {
		t.Parallel()
		f := startLaunchSession(t, wedge(t, "INSERT", 2))
		got := f.gxActionable(t)
		if !strings.Contains(got.Reason, "prompt not delivered") || len(got.Action) != 1 || got.Action[0] != launchPrompt {
			t.Errorf("actionable = %+v, want the prompt-not-delivered Actionable with the prompt as its Action", got)
		}
		if n := f.vimEvents(); n != 2 {
			t.Errorf("vim events = %d, want 2", n)
		}
		if run := readMeta(t, f.h, f.id)["run"].(float64); run != 2 {
			t.Errorf("run = %v, want exactly one restart", run)
		}
	})

	t.Run("NoFooterMeansNoRestartAndNoEvent", func(t *testing.T) {
		t.Parallel()
		f := startLaunchSession(t)
		f.waitForUserRecord(t)
		if n := f.vimEvents(); n != 0 {
			t.Errorf("vim events = %d, want 0", n)
		}
		if conv := f.currentConv(t); conv != f.conv {
			t.Errorf("conversation = %s, want the first (%s)", conv, f.conv)
		}
	})

	t.Run("CodexSessionShowsNoCheck", func(t *testing.T) {
		t.Parallel()
		h, _, _, id, _ := startFakeSessionWith(t, wire.CreateSessionRequest{Harness: "codex", Prompt: "fix the bug"},
			servertest.WithEnv("PATH="+servertest.FakeCodexDir(t)+string(os.PathListSeparator)+os.Getenv("PATH")))
		runBefore := readMeta(t, h, id)["run"]
		logPath := server.LogPath(h.StateDir)
		servertest.WaitFor(t, 30*time.Second, "the first prompt typed", func() bool {
			data, _ := os.ReadFile(logPath)
			return strings.Contains(string(data), "first prompt typed")
		})
		data, _ := os.ReadFile(logPath)
		if strings.Contains(string(data), vimEvent) {
			t.Error("vim event logged for a Codex session")
		}
		if got := readMeta(t, h, id)["run"]; got != runBefore {
			t.Errorf("run = %v, want %v: a Codex session is never restarted", got, runBefore)
		}
	})
}
