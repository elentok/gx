package server_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/syshealth"
	"github.com/elentok/gx/testutil"
)

const healthInterval = 5 * time.Millisecond

// startHealth starts a server on fake with a short read interval.
func startHealth(t *testing.T, fake *servertest.FakeSystem, interval time.Duration) *servertest.Harness {
	t.Helper()
	return servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.SystemHealth = fake.Read
		c.HealthPollInterval = interval
	})
}

func snapshotHealth(t *testing.T, h *servertest.Harness) wire.Snapshot {
	t.Helper()
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// healthWhen polls the snapshot until pred accepts its health.
func healthWhen(t *testing.T, h *servertest.Harness, what string, pred func(wire.Health) bool) wire.Health {
	t.Helper()
	var last wire.Health
	ok := testutil.Poll(10*time.Second, 5*time.Millisecond, func() bool {
		last = snapshotHealth(t, h).Health
		return pred(last)
	})
	if !ok {
		t.Fatalf("snapshot health = %+v, want %s", last, what)
	}
	return last
}

// waitReads waits until the fake has taken n readings, without asking the server.
func waitReads(t *testing.T, fake *servertest.FakeSystem, n int) {
	t.Helper()
	if !testutil.Poll(10*time.Second, time.Millisecond, func() bool { return fake.Reads() >= n }) {
		t.Fatalf("reads = %d, want %d", fake.Reads(), n)
	}
}

// Seam: no client asks while the watcher reads; the first snapshot already
// holds the one-minute average (story 22) and the state follows it (story 10).
func TestHealth_AverageIsReadyInTheFirstSnapshotAfterTwelveReadings(t *testing.T) {
	t.Parallel()
	fake := servertest.NewFakeSystem()
	fake.SetBusy(75)
	h := startHealth(t, fake, healthInterval)
	// The first reading has no CPU value, so 13 readings give 12 busy values.
	waitReads(t, fake, syshealth.AverageWindow+1)
	got := snapshotHealth(t, h).Health.CPU
	if got.State != wire.HealthStateHigh || got.Level != 3 {
		t.Fatalf("cpu = %+v, want high at level 3", got)
	}
}

// Seam: one high reading among a minute of low ones keeps the state normal (story 12).
func TestHealth_SingleSpikeKeepsStateNormal(t *testing.T) {
	t.Parallel()
	fake := servertest.NewFakeSystem()
	fake.SetBusy(5)
	h := startHealth(t, fake, healthInterval)
	healthWhen(t, h, "a known cpu", func(v wire.Health) bool { return v.CPU.State == wire.HealthStateNormal })
	fake.QueueBusy(100)
	waitReads(t, fake, fake.Reads()+4)
	if got := snapshotHealth(t, h).Health.CPU; got.State != wire.HealthStateNormal {
		t.Fatalf("cpu = %+v, want normal", got)
	}
}

// Seam: a level change publishes exactly one event (story 19).
func TestHealth_LevelChangePublishesOneEvent(t *testing.T) {
	t.Parallel()
	fake := servertest.NewFakeSystem()
	fake.SetMem(10)
	h := startHealth(t, fake, healthInterval)
	waitReads(t, fake, syshealth.AverageWindow+2)
	snap := snapshotHealth(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	fake.SetMem(30)
	healthWhen(t, h, "memory level 1", func(v wire.Health) bool { return v.Memory.Level == 1 })
	waitReads(t, fake, fake.Reads()+syshealth.AverageWindow)
	if n := servertest.HealthEvents(ch); n != 1 {
		t.Fatalf("health events = %d, want 1", n)
	}
}

// Seam: readings that move within one step publish nothing (story 20).
func TestHealth_MovementWithinAStepPublishesNoEvent(t *testing.T) {
	t.Parallel()
	fake := servertest.NewFakeSystem()
	fake.SetMem(10)
	h := startHealth(t, fake, healthInterval)
	waitReads(t, fake, syshealth.AverageWindow+2)
	snap := snapshotHealth(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	// A negative wait: flap memory across 40 readings, then prove no event went out.
	for i := range 40 {
		fake.SetMem(float64(10 + 4*(i%2)))
		waitReads(t, fake, fake.Reads()+1)
	}
	if n := servertest.HealthEvents(ch); n != 0 {
		t.Fatalf("health events = %d, want 0", n)
	}
}

// Seam: a failed read clears both one-minute windows, so the first meter
// after the error is not an average of readings from before it.
func TestHealth_ReadErrorClearsTheOneMinuteWindows(t *testing.T) {
	fake := servertest.NewFakeSystem()
	fake.SetMem(90)
	fake.SetBusy(90)
	h := startHealth(t, fake, healthInterval)
	waitReads(t, fake, syshealth.AverageWindow+2)
	healthWhen(t, h, "high memory and cpu", func(v wire.Health) bool {
		return v.Memory.State != wire.HealthStateNormal && v.CPU.State != wire.HealthStateNormal
	})
	fake.Fail(errors.New("boom"))
	healthWhen(t, h, "unknown after the error", func(v wire.Health) bool { return v.Memory.State == wire.HealthStateUnknown })
	fake.SetMem(10)
	fake.SetBusy(5)
	fake.Fail(nil)
	// The first known meters come from fresh windows: one low reading, not a stale average.
	got := healthWhen(t, h, "known memory and cpu", func(v wire.Health) bool {
		return v.Memory.State != wire.HealthStateUnknown && v.CPU.State != wire.HealthStateUnknown
	})
	if got.Memory.State != wire.HealthStateNormal || got.CPU.State != wire.HealthStateNormal {
		t.Fatalf("health = %+v, want memory and cpu normal", got)
	}
}

// Seam: unknown reasons (story 34).
func TestHealth_UnknownReasons(t *testing.T) {
	t.Parallel()
	t.Run("first interval is waiting", func(t *testing.T) {
		fake := servertest.NewFakeSystem()
		h := startHealth(t, fake, time.Hour)
		got := healthWhen(t, h, "memory read", func(v wire.Health) bool { return v.Memory.State == wire.HealthStateNormal })
		if got.CPU.State != wire.HealthStateUnknown || got.CPU.Reason != wire.HealthReasonWaiting {
			t.Fatalf("cpu = %+v, want unknown/waiting", got.CPU)
		}
	})
	for name, tc := range map[string]struct {
		err    error
		reason wire.HealthReason
	}{
		"unavailable source": {syshealth.ErrUnavailable, wire.HealthReasonUnavailable},
		"failing read":       {errors.New("boom"), wire.HealthReasonError},
	} {
		t.Run(name, func(t *testing.T) {
			fake := servertest.NewFakeSystem()
			fake.Fail(tc.err)
			h := startHealth(t, fake, healthInterval)
			got := healthWhen(t, h, "unknown "+string(tc.reason), func(v wire.Health) bool { return v.CPU.Reason == tc.reason })
			if got.CPU.State != wire.HealthStateUnknown || got.Memory.State != wire.HealthStateUnknown || got.Memory.Reason != tc.reason {
				t.Fatalf("health = %+v, want cpu and memory unknown/%s", got, tc.reason)
			}
		})
	}
}
