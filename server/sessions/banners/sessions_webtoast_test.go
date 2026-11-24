package banners_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// toastWithin reports whether a toast event arrives on evs within d.
func toastWithin(evs <-chan wire.Event, d time.Duration) (wire.Event, bool) {
	timeout := time.After(d)
	for {
		select {
		case ev, ok := <-evs:
			if !ok {
				return wire.Event{}, false
			}
			if ev.Type == wire.EventNotificationToast {
				return ev, true
			}
		case <-timeout:
			return wire.Event{}, false
		}
	}
}

func TestSessionWebToast_ADesignRoundNeedsDesignQuestionChecked(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		kinds []string
		want  bool
	}{
		{"design-question checked", []string{notifykind.DesignQuestion}, true},
		{"design-question unchecked", []string{notifykind.SessionAlert}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, ctx, evs, id, conv, path := startGrillingSessionIn(t, "proj:map", t.TempDir(), servertest.WithChannel("web-toast", true, tc.kinds))
			writeAssistantMessage(t, path, "Round one\n\n❓ **Q1** Which store?")
			reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "working"}, wire.SessionWorking)
			reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "idle"}, wire.SessionIdle)
			ev, got := toastWithin(evs, 3*servertest.SessionBannerDelay)
			if got != tc.want {
				t.Fatalf("toast = %+v, got %v, want %v", ev, got, tc.want)
			}
			if got && ev.Path != "/d/proj:map" {
				t.Errorf("toast path = %q, want /d/proj:map", ev.Path)
			}
		})
	}
}

func TestSessionWebToast_ADoneSessionNeedsSessionAlertChecked(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		kinds []string
		want  bool
	}{
		{"session-alert checked", []string{notifykind.SessionAlert}, true},
		{"session-alert unchecked", []string{notifykind.DesignQuestion}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, ctx, evs, id, conv := startBannerSession(t, servertest.WithChannel("web-toast", true, tc.kinds))
			reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, wire.SessionIdle)
			waitAlerts(h, 1)
			ev, got := toastWithin(evs, 3*servertest.SessionBannerDelay)
			if got != tc.want {
				t.Fatalf("toast = %+v, got %v, want %v", ev, got, tc.want)
			}
			if got && ev.Path != "/agents/"+id {
				t.Errorf("toast path = %q, want /agents/%s", ev.Path, id)
			}
		})
	}
}
