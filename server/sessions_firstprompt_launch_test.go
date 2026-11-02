package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	bin, home, config := t.TempDir(), t.TempDir(), t.TempDir()
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
	res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: dir, Harness: "claude", Prompt: launchPrompt})
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
	msgs, _, _ := transcript.ReadChat(transcript.PathIn(f.home, f.dir, f.conv))
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
