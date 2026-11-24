package banners_test

import (
	"slices"
	"testing"

	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/testutil/agentfake"
)

func TestSessionDesktop_SwitchOffGivesNoBannerNoSoundAndKeepsThePush(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startBannerSession(t, servertest.WithChannel("desktop", false, nil))
	reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, wire.SessionIdle)
	if got := waitPushes(h, 1); len(got) != 1 {
		t.Errorf("pushes = %+v, want one", got)
	}
	banners, sounds := waitAlerts(h, 0)
	if len(banners) != 0 || len(sounds) != 0 {
		t.Errorf("banners = %+v, sounds = %v, want none", banners, sounds)
	}
}

func TestSessionDesktop_ADoneSessionNeedsSessionAlertForBannerAndSound(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		kinds []string
		want  int
	}{
		{"session-alert checked", []string{notifykind.SessionAlert}, 1},
		{"only design-question checked", []string{notifykind.DesignQuestion}, 0},
		{"no kind checked", []string{}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, ctx, evs, id, conv := startBannerSession(t, servertest.WithChannel("desktop", true, tc.kinds))
			reach(ctx, t, h, evs, id, conv, agentfake.Control{State: "done"}, wire.SessionIdle)
			banners, sounds := waitAlerts(h, tc.want)
			if len(banners) != tc.want {
				t.Errorf("banners = %+v, want %d", banners, tc.want)
			}
			if wantSounds := map[bool][]sound.Name{true: {sound.Done}, false: nil}[tc.want == 1]; !slices.Equal(sounds, wantSounds) {
				t.Errorf("sounds = %v, want %v", sounds, wantSounds)
			}
		})
	}
}
