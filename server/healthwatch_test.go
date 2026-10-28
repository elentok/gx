package server_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/syshealth"
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

func snapshotHealth(t *testing.T, h *servertest.Harness) server.Snapshot {
	t.Helper()
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// healthWhen polls the snapshot until pred accepts its health.
func healthWhen(t *testing.T, h *servertest.Harness, what string, pred func(server.Health) bool) server.Health {
	t.Helper()
	var last server.Health
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if last = snapshotHealth(t, h).Health; pred(last) {
			return last
		}
	}
	t.Fatalf("snapshot health = %+v, want %s", last, what)
	return last
}

// waitReads waits until the fake has taken n readings, without asking the server.
func waitReads(t *testing.T, fake *servertest.FakeSystem, n int) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if fake.Reads() >= n {
			return
		}
	}
	t.Fatalf("reads = %d, want %d", fake.Reads(), n)
}

// healthEvents counts the health-changed events on stream until it is quiet.
func healthEvents(ch <-chan server.Event) int {
	n := 0
	for {
		select {
		case ev := <-ch:
			if ev.Type == server.EventHealthChanged {
				n++
			}
		case <-time.After(150 * time.Millisecond):
			return n
		}
	}
}

// Seam: no client asks while the watcher reads; the first snapshot already
// holds the one-minute average (story 22) and the state follows it (story 10).
func TestHealth_AverageIsReadyInTheFirstSnapshotAfterTwelveReadings(t *testing.T) {
	fake := servertest.NewFakeSystem()
	fake.SetBusy(75)
	h := startHealth(t, fake, healthInterval)
	// The first reading has no CPU value, so 13 readings give 12 busy values.
	waitReads(t, fake, syshealth.AverageWindow+1)
	got := snapshotHealth(t, h).Health.CPU
	if got.State != server.HealthStateHigh || got.Level != 3 {
		t.Fatalf("cpu = %+v, want high at level 3", got)
	}
}

// Seam: one high reading among a minute of low ones keeps the state normal (story 12).
func TestHealth_SingleSpikeKeepsStateNormal(t *testing.T) {
	fake := servertest.NewFakeSystem()
	fake.SetBusy(5)
	h := startHealth(t, fake, healthInterval)
	healthWhen(t, h, "a known cpu", func(v server.Health) bool { return v.CPU.State == server.HealthStateNormal })
	fake.QueueBusy(100)
	waitReads(t, fake, fake.Reads()+4)
	if got := snapshotHealth(t, h).Health.CPU; got.State != server.HealthStateNormal {
		t.Fatalf("cpu = %+v, want normal", got)
	}
}

// Seam: a level change publishes exactly one event (story 19).
func TestHealth_LevelChangePublishesOneEvent(t *testing.T) {
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
	healthWhen(t, h, "memory level 1", func(v server.Health) bool { return v.Memory.Level == 1 })
	waitReads(t, fake, fake.Reads()+syshealth.AverageWindow)
	if n := healthEvents(ch); n != 1 {
		t.Fatalf("health events = %d, want 1", n)
	}
}

// Seam: readings that move within one step publish nothing (story 20).
func TestHealth_MovementWithinAStepPublishesNoEvent(t *testing.T) {
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
	for i := range 40 {
		fake.SetMem(float64(10 + 4*(i%2)))
		time.Sleep(healthInterval)
	}
	if n := healthEvents(ch); n != 0 {
		t.Fatalf("health events = %d, want 0", n)
	}
}

// Seam: unknown reasons (story 34).
func TestHealth_UnknownReasons(t *testing.T) {
	t.Run("first interval is waiting", func(t *testing.T) {
		fake := servertest.NewFakeSystem()
		h := startHealth(t, fake, time.Hour)
		got := healthWhen(t, h, "memory read", func(v server.Health) bool { return v.Memory.State == server.HealthStateNormal })
		if got.CPU.State != server.HealthStateUnknown || got.CPU.Reason != server.HealthReasonWaiting {
			t.Fatalf("cpu = %+v, want unknown/waiting", got.CPU)
		}
	})
	for name, tc := range map[string]struct {
		err    error
		reason server.HealthReason
	}{
		"unavailable source": {syshealth.ErrUnavailable, server.HealthReasonUnavailable},
		"failing read":       {errors.New("boom"), server.HealthReasonError},
	} {
		t.Run(name, func(t *testing.T) {
			fake := servertest.NewFakeSystem()
			fake.Fail(tc.err)
			h := startHealth(t, fake, healthInterval)
			got := healthWhen(t, h, "unknown "+string(tc.reason), func(v server.Health) bool { return v.CPU.Reason == tc.reason })
			if got.CPU.State != server.HealthStateUnknown || got.Memory.State != server.HealthStateUnknown || got.Memory.Reason != tc.reason {
				t.Fatalf("health = %+v, want cpu and memory unknown/%s", got, tc.reason)
			}
		})
	}
}
