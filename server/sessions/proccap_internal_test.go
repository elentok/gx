package sessions

import (
	"slices"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/settings"
)

// The holder gets the cap counted at this launch plus the headroom read from
// the live settings at this launch.
func TestProcCapArgs_UsesLiveHeadroomAtEachLaunch(t *testing.T) {
	t.Parallel()
	set := &settings.Holder{}
	set.Store(&settings.Settings{Watchdog: config.WatchdogConfig{UlimitHeadroom: 1000}})
	mg := &Manager{settings: set, cfg: SessionsConfig{ProcCount: func() (int, error) { return 300, nil }}}

	if got, want := mg.procCapArgs(), []string{"--proc-cap", "1300"}; !slices.Equal(got, want) {
		t.Errorf("args = %v; want %v", got, want)
	}
	set.Store(&settings.Settings{Watchdog: config.WatchdogConfig{UlimitHeadroom: 50}})
	if got, want := mg.procCapArgs(), []string{"--proc-cap", "350"}; !slices.Equal(got, want) {
		t.Errorf("after a reload args = %v; want %v", got, want)
	}
}
