package server_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/testutil/agentfake"
)

func withAlerts(banners, sounds bool) func(*server.Config) {
	return func(c *server.Config) {
		c.Settings.AgentsBanners = banners
		c.Settings.AgentsSounds.Enabled = sounds
	}
}

func startBannerSession(t *testing.T, opts ...func(*server.Config)) (*servertest.Harness, context.Context, <-chan server.Event, string, string) {
	t.Helper()
	h, ctx, evs, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{Name: "fix-build"}, append([]func(*server.Config){withAlerts(true, true)}, opts...)...)
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "working"}, server.SessionWorking)
	return h, ctx, evs, id, conv
}

func reach(ctx context.Context, t *testing.T, h *servertest.Harness, evs <-chan server.Event, id, conv string, c agentfake.Control, state string) {
	t.Helper()
	report(t, conv, c)
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == state })
}

// waitAlerts waits past the banner delay and returns what was sent.
func waitAlerts(h *servertest.Harness, want int) ([]osnotify.Notification, []sound.Name) {
	deadline := time.Now().Add(3 * servertest.SessionBannerDelay)
	for time.Now().Before(deadline) && (len(h.Banners.Sent()) < want || len(h.Sounds.Played()) < want) {
		time.Sleep(10 * time.Millisecond)
	}
	if want == 0 {
		time.Sleep(2 * servertest.SessionBannerDelay)
	}
	return h.Banners.Sent(), h.Sounds.Played()
}

func TestSessionBanner_DoneSendsOneAfterDelay(t *testing.T) {
	h, ctx, evs, id, conv := startBannerSession(t)
	start := time.Now()
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, server.SessionIdle)
	banners, sounds := waitAlerts(h, 1)
	if elapsed := time.Since(start); elapsed < servertest.SessionBannerDelay {
		t.Errorf("banner after %v, want at least the delay", elapsed)
	}
	want := osnotify.Notification{Title: "proj · fix-build", Body: "Done", Group: "gx-agent-" + id}
	if len(banners) != 1 || banners[0] != want {
		t.Errorf("banners = %+v, want [%+v]", banners, want)
	}
	if !slices.Equal(sounds, []sound.Name{sound.Done}) {
		t.Errorf("sounds = %v, want [done]", sounds)
	}
}

func TestSessionBanner_BlockedBodyAndAttention(t *testing.T) {
	h, ctx, evs, id, conv := startBannerSession(t)
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "blocked", Kind: "permission", Message: "Bash(rm -rf build)"}, server.SessionBlocked)
	banners, sounds := waitAlerts(h, 1)
	if len(banners) != 1 || banners[0].Body != "Blocked: permission — Bash(rm -rf build)" {
		t.Errorf("banners = %+v, want one Blocked body", banners)
	}
	if !slices.Equal(sounds, []sound.Name{sound.Attention}) {
		t.Errorf("sounds = %v, want [attention]", sounds)
	}
}

func TestSessionBanner_DoneThenActionableMerges(t *testing.T) {
	h, ctx, evs, id, conv := startBannerSession(t)
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, server.SessionIdle)
	res, err := h.Client.MarkSessionActionable(ctx, server.MarkActionableRequest{ID: id, Reason: "review the plan"})
	if err != nil || res.Refused {
		t.Fatalf("mark actionable = %+v, %v", res, err)
	}
	banners, sounds := waitAlerts(h, 1)
	if len(banners) != 1 || banners[0].Body != "review the plan" {
		t.Errorf("banners = %+v, want one with the reason", banners)
	}
	if !slices.Equal(sounds, []sound.Name{sound.Attention}) {
		t.Errorf("sounds = %v, want [attention]", sounds)
	}
}

func TestSessionBanner_BlockedThenWorkingDrops(t *testing.T) {
	h, ctx, evs, id, conv := startBannerSession(t)
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "blocked", Kind: "permission"}, server.SessionBlocked)
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "working"}, server.SessionWorking)
	if banners, sounds := waitAlerts(h, 0); len(banners) != 0 || len(sounds) != 0 {
		t.Errorf("banners = %+v, sounds = %v; want none", banners, sounds)
	}
}

func TestSessionBanner_OnScreenInFocusedTUISkips(t *testing.T) {
	h, ctx, evs, id, conv := startBannerSession(t)
	openTUI(ctx, t, h, "tui-1")
	reportFocus(ctx, t, h, server.FocusReport{Client: "tui-1", Focused: true, Session: id, TermProgram: "ghostty"})
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, server.SessionIdle)
	if banners, sounds := waitAlerts(h, 0); len(banners) != 0 || len(sounds) != 0 {
		t.Errorf("banners = %+v, sounds = %v; want none", banners, sounds)
	}
}

func TestSessionBanner_ClickFocusesThroughReportedTerminal(t *testing.T) {
	h, ctx, evs, id, conv := startBannerSession(t)
	dropTUI := openTUI(ctx, t, h, "tui-1")
	reportFocus(ctx, t, h, server.FocusReport{Client: "tui-1", Focused: true, TermProgram: "iTerm.app"})
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, server.SessionIdle)
	banners, _ := waitAlerts(h, 1)
	if len(banners) != 1 || banners[0].Execute != "gx agents focus "+id || banners[0].Activate != "com.googlecode.iterm2" {
		t.Fatalf("banners = %+v, want focus + iTerm activate", banners)
	}

	dropTUI()
	waitFor(t, func() bool {
		res, err := h.Client.ReportFocus(ctx, server.FocusReport{Client: "tui-1"})
		return err == nil && res.Refused
	})
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "working"}, server.SessionWorking)
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, server.SessionIdle)
	banners, _ = waitAlerts(h, 2)
	if len(banners) != 2 || banners[1].Execute != "" || banners[1].Activate != "com.googlecode.iterm2" {
		t.Errorf("banners = %+v, want only activate with no TUI", banners)
	}
}

func TestSessionBanner_SwitchesOff(t *testing.T) {
	for _, tc := range []struct {
		name            string
		banners, sounds bool
	}{{"banners off", false, true}, {"sounds off", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			h, ctx, evs, id, conv := startBannerSession(t, withAlerts(tc.banners, tc.sounds))
			reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, server.SessionIdle)
			time.Sleep(2 * servertest.SessionBannerDelay)
			if got := len(h.Banners.Sent()) == 1; got != tc.banners {
				t.Errorf("banner sent = %v, want %v", got, tc.banners)
			}
			if got := len(h.Sounds.Played()) == 1; got != tc.sounds {
				t.Errorf("sound played = %v, want %v", got, tc.sounds)
			}
		})
	}
}

// openTUI opens a drained event stream as client, returning its close.
func openTUI(ctx context.Context, t *testing.T, h *servertest.Harness, client string) context.CancelFunc {
	t.Helper()
	tuiCtx, drop := context.WithCancel(ctx)
	t.Cleanup(drop)
	evs, err := h.Client.EventsAs(tuiCtx, 0, client)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for range evs {
		}
	}()
	return drop
}
