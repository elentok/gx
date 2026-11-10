package webflows

import (
	"os"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/vitefake"
)

func (r *running) devBlock(t *testing.T) wire.DevStatus {
	t.Helper()
	var snap wire.Snapshot
	r.Web(t).Get(t, "/v1/snapshot", &snap)
	return snap.Dev
}

func (r *running) waitDevState(t *testing.T, want wire.DevState) wire.DevStatus {
	t.Helper()
	var got wire.DevStatus
	if !testutil.Poll(30*time.Second, 100*time.Millisecond, func() bool {
		got = r.devBlock(t)
		return got.State == want
	}) {
		t.Fatalf("dev state = %q (reason %q), want %q", got.State, got.Reason, want)
	}
	return got
}

func TestDevModeScenario_EveryShapeShowsItsState(t *testing.T) {
	for _, w := range []struct {
		shape string
		want  wire.DevState
	}{
		{"off", wire.DevStateOff},
		{"installing", wire.DevStateInstalling},
		{"starting", wire.DevStateStarting},
		{"running", wire.DevStateRunning},
		{"down", wire.DevStateRunning},
	} {
		t.Run(w.shape, func(t *testing.T) {
			r := startWith(t, "dev-mode", nil, "--shape", w.shape)
			dev := r.waitDevState(t, w.want)
			if !dev.HasCheckout || dev.Checkout == "" {
				t.Errorf("dev = %+v, want a Dev checkout", dev)
			}
		})
	}
}

func TestDevModeScenario_DefaultShapeIsRunning(t *testing.T) {
	r := start(t, "dev-mode")
	r.waitDevState(t, wire.DevStateRunning)
}

func TestDevModeScenario_DownFollowsTheControlFile(t *testing.T) {
	r := startWith(t, "dev-mode", nil, "--shape", "down")
	r.waitDevState(t, wire.DevStateRunning)
	if r.DevControlFile == "" {
		t.Fatal("output names no dev control file")
	}
	if err := vitefake.WriteControl(r.DevControlFile, vitefake.Control{Vite: "crash"}); err != nil {
		t.Fatal(err)
	}
	dev := r.waitDevState(t, wire.DevStateDown)
	if dev.Reason == "" {
		t.Errorf("down has no reason: %+v", dev)
	}
}

func TestDevModeScenario_NoCheckoutReportsNone(t *testing.T) {
	r := startWith(t, "dev-mode", nil, "--shape", "no-checkout")
	dev := r.devBlock(t)
	if dev.HasCheckout || dev.Checkout != "" || dev.State != wire.DevStateOff || dev.Reason == "" {
		t.Errorf("dev = %+v, want off with no checkout and a reason", dev)
	}
	if _, err := os.Stat(r.DevControlFile); err != nil {
		t.Errorf("control file: %v", err)
	}
}
