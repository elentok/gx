package settings_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
)

// Seam: a web-trigger reload with the watcher on runs once and pops no OS
// banner; a command reload still pops one; a later hand edit still reloads.
func TestConfigWeb_ReloadsOnceWithoutBanner(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h, path, store, evs := watchHarness(t, ctx)
	holder := h.Server.Settings()

	writeWatched(t, path, store, 4)
	res := holder.Reload(settings.TriggerWeb)
	if res.Refused || !slices.Contains(res.Changed, "execution-queue.max-agents") {
		t.Fatalf("web reload = %+v, want max-agents changed", res)
	}
	// The web save itself tells other open pages (one event, no banner).
	timeout := time.After(5 * time.Second)
	for got := false; !got; {
		select {
		case ev := <-evs:
			got = ev.Type == wire.EventConfigReloaded
		case <-timeout:
			t.Fatal("a web save published no config-reloaded event")
		}
	}
	// The watcher would reload within a few debounces; it must stay silent.
	noReloadEvent(t, evs, "the watcher after a web save")
	if last := holder.Status().LastReload; last == nil || last.Trigger != settings.TriggerWeb {
		t.Errorf("last reload = %+v, want the web reload still the latest", last)
	}
	if n := len(h.Banners.Sent()); n != 0 {
		t.Errorf("web reload sent %d banners, want none", n)
	}

	reload(t, h)
	servertest.WaitFor(t, 5*time.Second, "a command banner", func() bool { return len(h.Banners.Sent()) == 1 })

	writeWatched(t, path, store, 4)
	holder.Reload(settings.TriggerWeb)
	writeWatched(t, path, store, 5)
	servertest.WaitFor(t, 5*time.Second, "the watcher's reload of the hand edit", func() bool {
		last := holder.Status().LastReload
		return last != nil && last.Trigger == settings.TriggerWatch
	})
}
