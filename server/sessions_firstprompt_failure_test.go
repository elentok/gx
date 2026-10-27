package server_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/testutil/agentfake"
)

const firstPromptStartTimeout = 300 * time.Millisecond

// startFailingFirstPrompt's extra entries are KEY=VALUE env for the server.
func startFailingFirstPrompt(t *testing.T, prompt string, extraEnv ...string) firstPromptFixture {
	t.Helper()
	h, ctx, evs, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{Prompt: prompt}, func(c *server.Config) {
		c.PromptGrace = firstPromptGrace
		c.FirstPromptStartTimeout = firstPromptStartTimeout
	}, withAlerts(true, true), servertest.WithEnv(append([]string{agentfake.ProgramInputEnv + "=1"}, extraEnv...)...))
	return firstPromptFixture{h: h, ctx: ctx, evs: evs, id: id, conv: conv}
}

type firstPromptFixture struct {
	h    *servertest.Harness
	ctx  context.Context
	evs  <-chan server.Event
	id   string
	conv string
}

// gxActionable waits for the session's gx-raised Actionable.
func (f firstPromptFixture) gxActionable(t *testing.T) server.SessionActionable {
	t.Helper()
	return waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Actionable.GxRaised }).Actionable
}

func TestFirstPromptFailure_StillStartingRaisesActionable(t *testing.T) {
	f := startFailingFirstPrompt(t, "fix the bug")
	got := f.gxActionable(t)
	if !slices.Equal(got.Action, []string{"fix the bug"}) || !strings.Contains(got.Reason, "still starting") {
		t.Errorf("actionable = %+v, want the prompt as a one-step Action and a still-starting reason", got)
	}
	banners, sounds := waitAlerts(f.h, 1)
	if len(banners) != 1 || banners[0].Body != got.Reason || !slices.Contains(sounds, sound.Attention) {
		t.Errorf("banners = %+v, sounds = %v, want one banner with the reason and the attention sound", banners, sounds)
	}
}

func TestFirstPromptFailure_BlockedTimeDoesNotCountAsStarting(t *testing.T) {
	f := startFailingFirstPrompt(t, "fix the bug", agentfake.ProgramTrustEnv+"=1")
	time.Sleep(3 * firstPromptStartTimeout)
	snap, err := f.h.Client.Snapshot(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Sessions {
		if s.ID == f.id && s.Actionable.GxRaised {
			t.Errorf("actionable = %+v while Blocked, want none", s.Actionable)
		}
	}
}

func TestFirstPromptFailure_NeverStartedClearsLeftoverText(t *testing.T) {
	f := startFailingFirstPrompt(t, "fix the bug")
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Millisecond):
				_ = agentfake.SendProgramControl(f.conv, agentfake.Control{Action: agentfake.ActionSwallowEnter})
			}
		}
	}()
	report(t, f.conv, agentfake.Control{State: "idle"})
	got := f.gxActionable(t)
	if want := "agent never started"; got.Reason != want {
		t.Errorf("reason = %q, want %q (the leftover text cleared)", got.Reason, want)
	}
	if !slices.Equal(got.Action, []string{"fix the bug"}) {
		t.Errorf("action = %v, want the prompt as a one-step Action", got.Action)
	}
	holder := waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
	if rows, _, err := holder.Screen(0); err != nil || strings.Contains(strings.Join(rows, "\n"), "fix the bug") {
		t.Errorf("screen = %q, %v, want the input cleared", rows, err)
	}
}

func TestFirstPromptFailure_GxRaisedSurvivesInputAndAgentRaisedDoesNot(t *testing.T) {
	f := startFailingFirstPrompt(t, "fix the bug")
	f.gxActionable(t)

	c := waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
	att, err := c.Attach()
	if err != nil {
		t.Fatal(err)
	}
	defer att.Close()
	if err := att.SendKey(screen.Key{Code: 'x', Text: "x"}); err != nil {
		t.Fatal(err)
	}
	// A later status change orders the check after the holder saw the input.
	report(t, f.conv, agentfake.Control{State: "working"})
	if got := waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking }); !got.Actionable.GxRaised {
		t.Errorf("actionable = %+v after input, want the gx-raised one kept", got.Actionable)
	}

	if res, err := f.h.Client.MarkSessionActionable(f.ctx, server.MarkActionableRequest{ID: f.id, Reason: "agent says"}); err != nil || res.Refused || res.Session.Actionable.GxRaised {
		t.Fatalf("mark actionable = %+v, %v", res, err)
	}
	if err := att.SendKey(screen.Key{Code: 'y', Text: "y"}); err != nil {
		t.Fatal(err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Actionable.Reason == "" })
}
