package serverflows_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/devvite/vitetest"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// devEvents collects the Dev events of one rig from now on.
type devEvents struct {
	t   *testing.T
	evs <-chan wire.Event
}

func subscribeDevEvents(t *testing.T, r *devRig) *devEvents {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	snap, err := r.h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := r.h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	return &devEvents{t: t, evs: evs}
}

// expect waits for an event of the type.
func (d *devEvents) expect(typ string) {
	d.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	servertest.WaitForEvent(ctx, d.t, d.evs, typ, "")
}

// none fails if an event of the type arrives within a short window after the
// caller has already seen the effect of the change it checks.
func (d *devEvents) none(typ string) {
	d.t.Helper()
	timer := time.After(300 * time.Millisecond)
	for {
		select {
		case ev := <-d.evs:
			if ev.Type == typ {
				d.t.Fatalf("unexpected %s event", typ)
			}
		case <-timer:
			return
		}
	}
}

func TestDevMode_ReloadAtReadyAndAtOff(t *testing.T) {
	checkout := vitetest.WriteDevCheckout(t, true)
	r := startDevRig(t, checkout, false)
	evs := subscribeDevEvents(t, r)

	r.setKey(t, checkout, true)
	evs.expect(wire.EventDevChanged)
	evs.none(wire.EventReload) // still starting
	r.fake.Release()
	evs.expect(wire.EventReload)

	r.setKey(t, checkout, false)
	evs.expect(wire.EventReload)
}

// The observer publishes the reload before dev-changed, so a dev-changed seen
// after the reload is the one for starting -> running.
func TestDevMode_DevChangedAtReady(t *testing.T) {
	checkout := vitetest.WriteDevCheckout(t, true)
	r := startDevRig(t, checkout, true)
	servertest.WaitFor(t, 10*time.Second, "the starting state", func() bool {
		return devSnapshot(t, r.h).State == wire.DevStateStarting
	})
	evs := subscribeDevEvents(t, r)

	r.fake.Release()
	evs.expect(wire.EventReload)
	evs.expect(wire.EventDevChanged)
}

func TestDevMode_LogLinesPublishNothing(t *testing.T) {
	checkout := vitetest.WriteDevCheckout(t, true)
	r := startDevRig(t, checkout, true)
	r.fake.Release()
	r.waitPage(t, vitetest.ViteStubPage)
	evs := subscribeDevEvents(t, r)

	r.fake.Log("hot update")
	servertest.WaitFor(t, 10*time.Second, "the log line in the snapshot", func() bool {
		for _, l := range devSnapshot(t, r.h).LogTail {
			if strings.Contains(l, "hot update") {
				return true
			}
		}
		return false
	})
	evs.none(wire.EventDevChanged)
	evs.none(wire.EventReload)
}

func TestDevMode_ReloadAtReadyAfterDevDirChange(t *testing.T) {
	first := vitetest.WriteDevCheckout(t, true)
	second := vitetest.WriteDevCheckout(t, true)
	r := startDevRig(t, first, true)
	r.fake.Release()
	r.waitPage(t, vitetest.ViteStubPage)
	evs := subscribeDevEvents(t, r)

	r.setKey(t, second, true)
	evs.expect(wire.EventReload)
	if dev := devSnapshot(t, r.h); dev.State != wire.DevStateRunning {
		t.Fatalf("dev = %+v, want running", dev)
	}
}

// An open settings page enables its dev-frontend row from has_checkout, so a
// dev-dir write must announce itself even while the supervisor stays off.
func TestDevMode_DevDirWriteTurningOnHasCheckoutPublishesDevChanged(t *testing.T) {
	checkout := vitetest.WriteDevCheckout(t, true)
	r := startDevRig(t, filepath.Join(t.TempDir(), "missing"), false)
	if dev := devSnapshot(t, r.h); dev.HasCheckout {
		t.Fatalf("dev = %+v, want no checkout", dev)
	}
	evs := subscribeDevEvents(t, r)

	r.setKey(t, checkout, false)
	evs.expect(wire.EventDevChanged)
	if dev := devSnapshot(t, r.h); !dev.HasCheckout {
		t.Fatalf("dev = %+v, want a checkout", dev)
	}
}

func TestDevMode_CrashPublishesDevChangedButNoReload(t *testing.T) {
	checkout := vitetest.WriteDevCheckout(t, true)
	r := startDevRig(t, checkout, true)
	r.fake.Release()
	r.waitPage(t, vitetest.ViteStubPage)
	evs := subscribeDevEvents(t, r)

	r.fake.Crash()
	evs.expect(wire.EventDevChanged)
	servertest.WaitFor(t, 10*time.Second, "the down state", func() bool {
		return devSnapshot(t, r.h).State == wire.DevStateDown
	})
	evs.none(wire.EventReload)
}

func TestDevMode_ReloadAtReadyAfterRestartVite(t *testing.T) {
	checkout := vitetest.WriteDevCheckout(t, true)
	r := startDevRig(t, checkout, true)
	r.fake.Release()
	r.waitPage(t, vitetest.ViteStubPage)
	r.fake.Crash()
	servertest.WaitFor(t, 10*time.Second, "the down state", func() bool {
		return devSnapshot(t, r.h).State == wire.DevStateDown
	})
	evs := subscribeDevEvents(t, r)

	if res := r.restart(t); res.Refused {
		t.Fatalf("restart refused: %+v", res)
	}
	evs.expect(wire.EventReload)
}
