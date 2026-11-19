package sessions

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/settings"
)

func TestCanPrefill(t *testing.T) {
	old := claudeSupportsPrefill
	t.Cleanup(func() { claudeSupportsPrefill = old })
	supported, mg, claude := true, &Manager{}, string(harness.Claude)
	claudeSupportsPrefill = func([]string) bool { return supported }
	tests := []struct {
		name            string
		meta            Meta
		supported, want bool
	}{
		{"plain text on a claude that knows the flag", Meta{Harness: claude, FirstPrompt: "fix the bug"}, true, true},
		{"claude that does not know the flag", Meta{Harness: claude, FirstPrompt: "fix the bug"}, false, false},
		{"no prompt", Meta{Harness: claude}, true, false},
		{"slash command", Meta{Harness: claude, FirstPrompt: "/gx-design x"}, true, false},
		{"images go in as pastes", Meta{Harness: claude, FirstPrompt: "look", FirstImages: []string{"a"}}, true, false},
		{"too long for argv", Meta{Harness: claude, FirstPrompt: strings.Repeat("x", harness.PrefillMaxBytes+1)}, true, false},
		{"another harness", Meta{Harness: string(harness.Codex), FirstPrompt: "fix the bug"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			supported = tt.supported
			if got := mg.canPrefill(&tt.meta); got != tt.want {
				t.Errorf("canPrefill = %v, want %v", got, tt.want)
			}
		})
	}
}

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
