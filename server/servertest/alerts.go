package servertest

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/testutil/agentfake"
)

// WithAlerts turns the agent banners and sounds on or off.
func WithAlerts(banners, sounds bool) func(*server.Config) {
	return func(c *server.Config) {
		c.Settings.AgentsBanners = banners
		c.Settings.AgentsSounds.Enabled = sounds
	}
}

// StartBannerSession starts a fake session named fix-build with banners and
// sounds on, and brings it to Working.
func StartBannerSession(t *testing.T, opts ...func(*server.Config)) (*Harness, context.Context, <-chan wire.Event, string, string) {
	t.Helper()
	h, ctx, evs, id, conv := StartFakeSessionWith(t, wire.CreateSessionRequest{Name: "fix-build"}, append([]func(*server.Config){WithAlerts(true, true)}, opts...)...)
	Reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "working"}, wire.SessionWorking)
	return h, ctx, evs, id, conv
}

// Reach reports c to the fake agent and waits for the session to reach state.
func Reach(ctx context.Context, t *testing.T, h *Harness, evs <-chan wire.Event, id, conv string, c agentfake.Control, state string) {
	t.Helper()
	Report(t, conv, c)
	WaitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == state })
}

// WaitAlerts waits past the banner delay and returns what was sent.
func WaitAlerts(h *Harness, want int) ([]osnotify.Notification, []sound.Name) {
	Poll(3*SessionBannerDelay, func() bool {
		return len(h.Banners.Sent()) >= want && len(h.Sounds.Played()) >= want
	})
	if want == 0 {
		time.Sleep(2 * SessionBannerDelay) // negative wait: an alert would fire within the banner delay
	}
	return h.Banners.Sent(), h.Sounds.Played()
}

// OpenTUI opens a flushed event stream as client, returning its close.
func OpenTUI(ctx context.Context, t *testing.T, h *Harness, client string) context.CancelFunc {
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

// ReportFocus sends a focus report and fails on a refusal.
func ReportFocus(ctx context.Context, t *testing.T, h *Harness, r wire.FocusReport) {
	t.Helper()
	res, err := h.Client.ReportFocus(ctx, r)
	if err != nil || res.Refused {
		t.Fatalf("report focus = %+v, %v", res, err)
	}
}

// IdleWithAction brings the session Idle and raises an Actionable carrying
// action.
func IdleWithAction(ctx context.Context, t *testing.T, h *Harness, evs <-chan wire.Event, id, conv string, action ...string) {
	t.Helper()
	Report(t, conv, agentfake.Control{State: "idle"})
	WaitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	res, err := h.Client.MarkSessionActionable(ctx, wire.MarkActionableRequest{ID: id, Reason: "ready", Action: action})
	if err != nil || res.Refused {
		t.Fatalf("mark actionable = %+v, %v", res, err)
	}
}

// ScreenHas reports whether the holder's screen shows text.
func ScreenHas(t *testing.T, c *nativerunner.HolderClient, text string) bool {
	t.Helper()
	rows, _, err := c.Screen(0)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(strings.Join(rows, "\n"), text)
}

// WaitForScreen waits for the holder's screen to show text.
func WaitForScreen(t *testing.T, c *nativerunner.HolderClient, text string) {
	t.Helper()
	WaitFor(t, 10*time.Second, fmt.Sprintf("the screen to show %q", text), func() bool { return ScreenHas(t, c, text) })
}
