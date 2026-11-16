package rest_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/rest"
)

// Seam: a running Break count wakes the runner when the idle gap reaches the
// Break length, and no count means no such wake.
func TestNextWake_IdleResetOfBreakCount(t *testing.T) {
	t.Parallel()
	cfg := breakCfg()
	now := start()
	last := now.Add(-time.Minute)
	st := rest.State{WorkCount: 1, LastActive: last}
	want := last.Add(time.Duration(cfg.Break.BreakMinutes) * time.Minute)
	if got, ok := rest.NextWake(cfg, st, now); !ok || !got.Equal(want) {
		t.Errorf("wake = %v, %v, want %v", got, ok, want)
	}
	if _, ok := rest.NextWake(cfg, rest.State{LastActive: last}, now); ok {
		t.Error("a zero Break count has a wake")
	}
}

// Seam: an idle Break warning ends and publishes once the idle gap passes.
func TestRunner_IdleGapEndsBreakWarning(t *testing.T) {
	t.Parallel()
	g := openRig(t, t.TempDir(), start(), breakCfg())
	for range 2 { // one below Work time: the Break warning shows
		if _, err := g.Active(); err != nil {
			t.Fatal(err)
		}
		g.now = g.now.Add(time.Minute)
	}
	if g.View().Warning == nil {
		t.Fatal("no Break warning before the idle gap")
	}
	evs, cancel, err := g.events.Subscribe(g.events.CurrentSeq())
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	g.now = g.now.Add(time.Duration(g.cfg.Break.BreakMinutes) * time.Minute)
	if _, err := g.Tick(); err != nil {
		t.Fatal(err)
	}
	if g.View().Warning != nil {
		t.Errorf("warning = %+v after the idle gap, want none", g.View().Warning)
	}
	if ev := <-evs; ev.Type != wire.EventRestChanged {
		t.Errorf("event = %q, want %q", ev.Type, wire.EventRestChanged)
	}
}
