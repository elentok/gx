package promptflows_test

import (
	"slices"
	"testing"

	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/sound"
)

// A Design round with only design-question checked for Desktop gives a banner
// with a sound; the Done-session kind is not needed.
func TestDesignQuestion_DesktopBannerNeedsOnlyDesignQuestion(t *testing.T) {
	t.Parallel()
	f := rigs.StartPromptSessionWith(t, rigs.FirstPromptGrace, servertest.WithAlerts(true, true),
		servertest.WithChannel("desktop", true, []string{notifykind.DesignQuestion}))
	r := &questionRig{f: f}
	r.askRound(t, 1)
	banners, sounds := waitAlerts(f.H, 1)
	if len(banners) != 1 || !slices.Equal(sounds, []sound.Name{sound.Attention}) {
		t.Errorf("banners = %+v, sounds = %v, want one banner and one sound", banners, sounds)
	}
}
