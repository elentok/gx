package server_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
)

// diskMeterWhen polls the snapshot until pred accepts the disk meter.
func (r *diskRig) diskMeterWhen(t *testing.T, what string, pred func(server.MeterHealth) bool) server.MeterHealth {
	t.Helper()
	var last server.MeterHealth
	ok := testutil.Poll(10*time.Second, 10*time.Millisecond, func() bool {
		snap, err := r.h.Client.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		last = snap.Health.Disk
		return pred(last)
	})
	if !ok {
		t.Fatalf("snapshot disk meter = %+v, want %s", last, what)
	}
	return last
}

// diskByID finds a disk in the report; the scratch project always adds a "fake" one.
func diskByID(rep server.DiskReport, id string) server.DiskStatus {
	for _, d := range rep.Disks {
		if d.ID == id {
			return d
		}
	}
	return server.DiskStatus{}
}

func diskIs(state server.HealthState, level int) func(server.MeterHealth) bool {
	return func(m server.MeterHealth) bool { return m.State == state && m.Level == level }
}

// Seam: the meter's state follows the disk watcher's flags (stories 4 and 15),
// and the level is the percent used.
func TestDiskHealth_StateAgreesWithTheDiskStatusFlags(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := startDiskRig(t, map[string]string{"proj": repo}, 20*gb)
	r.disks.PutSized(repo, "d1", "/vol1", 30*gb, 100*gb)
	// 70% used.
	r.diskMeterWhen(t, "normal at level 3", diskIs(server.HealthStateNormal, 3))

	r.disks.SetFree(repo, 15*gb)
	r.diskMeterWhen(t, "high at level 4", diskIs(server.HealthStateHigh, 4))
	rep, err := r.h.Client.DiskStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d := diskByID(rep, "d1"); !d.Warned || d.Held {
		t.Fatalf("disk = %+v, want warned, not held", d)
	}

	r.disks.SetFree(repo, 5*gb)
	r.diskMeterWhen(t, "critical at level 4", diskIs(server.HealthStateCritical, 4))
	rep, err = r.h.Client.DiskStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d := diskByID(rep, "d1"); !d.Held {
		t.Fatalf("disk = %+v, want held", d)
	}
}

// Seam: with two disks, one warned, the meter reads high (story 16).
func TestDiskHealth_WorstDiskWins(t *testing.T) {
	t.Parallel()
	repo, goCache := t.TempDir(), t.TempDir()
	r := startDiskRig(t, map[string]string{"proj": repo}, 20*gb, func(c *server.Config) {
		c.GoCacheDir = func() (string, error) { return goCache, nil }
	})
	r.disks.PutSized(repo, "d1", "/vol1", 50*gb, 100*gb)
	r.disks.PutSized(goCache, "d2", "/vol2", 12*gb, 100*gb)
	// The warned disk is 88% used, the other 50%. The meter may start from the
	// scratch project's disk, so 88% is past every level edge plus its margin.
	r.diskMeterWhen(t, "high at level 4", diskIs(server.HealthStateHigh, 4))
}

// Seam: no disk read yet is unknown and waiting.
func TestDiskHealth_NoDiskIsWaiting(t *testing.T) {
	t.Parallel()
	r := startDiskRig(t, nil, 20*gb, func(c *server.Config) {
		c.FreeSpace = func(string) (server.DiskReading, error) { return server.DiskReading{}, errors.New("no disk") }
	})
	time.Sleep(quiet) // negative wait: failed readings never leave the waiting state
	got := r.diskMeterWhen(t, "waiting", func(m server.MeterHealth) bool { return true })
	if got.State != server.HealthStateUnknown || got.Reason != server.HealthReasonWaiting {
		t.Fatalf("disk = %+v, want unknown, waiting", got)
	}
}

// Seam: the disk status route lists total bytes for each disk.
func TestDiskStatus_ListsTotalBytes(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := startDiskRig(t, map[string]string{"proj": repo}, 20*gb)
	r.disks.PutSized(repo, "d1", "/vol1", 30*gb, 100*gb)
	r.diskMeterWhen(t, "a disk", diskIs(server.HealthStateNormal, 3))
	rep, err := r.h.Client.DiskStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d := diskByID(rep, "d1"); d.TotalBytes != 100*gb {
		t.Fatalf("disk = %+v, want total 100G", d)
	}
}

// Seam: the health event fires only on a disk level or state change.
func TestDiskHealth_EventOnlyOnLevelOrStateChange(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := startDiskRig(t, map[string]string{"proj": repo}, 20*gb)
	r.disks.PutSized(repo, "d1", "/vol1", 30*gb, 100*gb)
	r.diskMeterWhen(t, "normal at level 3", diskIs(server.HealthStateNormal, 3))
	snap, err := r.h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := r.h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	r.disks.SetFree(repo, 28*gb) // 72% used: same level, still normal
	r.waitReading(t, "d1", 28*gb)
	r.disks.SetFree(repo, 15*gb) // 85% used and warned
	r.diskMeterWhen(t, "high at level 4", diskIs(server.HealthStateHigh, 4))
	if n := healthEvents(ch); n != 1 {
		t.Fatalf("health events = %d, want 1", n)
	}
}
