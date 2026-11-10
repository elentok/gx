package sessions

import (
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/settings"
)

// launch hands this ctx to the runner's launch prompt, so a stop reaches an
// in-flight launch only if close ends it.
func TestSessionWatches_CloseCancelsTheLaunchContext(t *testing.T) {
	t.Parallel()
	w := NewSessionWatches()
	ctx, release := w.Context()
	defer release()

	w.Close()

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("launch context still live after close")
	}
}

// The holder gets the cap counted at this launch plus the headroom read from
// the live settings at this launch.
func TestProcCapArgs_UsesLiveHeadroomAtEachLaunch(t *testing.T) {
	t.Parallel()
	set := &settings.Holder{}
	set.Store(&settings.Settings{Watchdog: config.WatchdogConfig{UlimitHeadroom: 1000}})
	mg := &Manager{settings: set, cfg: SessionsConfig{ProcCount: func() (int, error) { return 300, nil }}}

	if got, want := procCapArgs(mg), []string{"--proc-cap", "1300"}; !slices.Equal(got, want) {
		t.Errorf("args = %v; want %v", got, want)
	}
	set.Store(&settings.Settings{Watchdog: config.WatchdogConfig{UlimitHeadroom: 50}})
	if got, want := procCapArgs(mg), []string{"--proc-cap", "350"}; !slices.Equal(got, want) {
		t.Errorf("after a reload args = %v; want %v", got, want)
	}
}
