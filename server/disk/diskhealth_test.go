package disk_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func diskIs(state wire.HealthState, level int) func(wire.MeterHealth) bool {
	return func(m wire.MeterHealth) bool { return m.State == state && m.Level == level }
}

// Seam: the meter's state follows the disk watcher's flags (stories 4 and 15),
// and the level is the percent used.
func TestDiskHealth_StateAgreesWithTheDiskStatusFlags(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := servertest.StartDiskRig(t, map[string]string{"proj": repo}, 20*gb)
	r.Disks.PutSized(repo, "d1", "/vol1", 30*gb, 100*gb)
	// 70% used.
	r.DiskMeterWhen(t, "normal at level 3", diskIs(wire.HealthStateNormal, 3))

	r.Disks.SetFree(repo, 15*gb)
	r.DiskMeterWhen(t, "high at level 4", diskIs(wire.HealthStateHigh, 4))
	rep, err := r.H.Client.Admin.DiskStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d := servertest.DiskByID(rep, "d1"); !d.Warned || d.Held {
		t.Fatalf("disk = %+v, want warned, not held", d)
	}

	r.Disks.SetFree(repo, 5*gb)
	r.DiskMeterWhen(t, "critical at level 4", diskIs(wire.HealthStateCritical, 4))
	rep, err = r.H.Client.Admin.DiskStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d := servertest.DiskByID(rep, "d1"); !d.Held {
		t.Fatalf("disk = %+v, want held", d)
	}
}

// Seam: with two disks, one warned, the meter reads high (story 16).
func TestDiskHealth_WorstDiskWins(t *testing.T) {
	t.Parallel()
	repo, goCache := t.TempDir(), t.TempDir()
	r := servertest.StartDiskRig(t, map[string]string{"proj": repo}, 20*gb, func(c *server.Config) {
		c.GoCacheDir = func() (string, error) { return goCache, nil }
	})
	r.Disks.PutSized(repo, "d1", "/vol1", 50*gb, 100*gb)
	r.Disks.PutSized(goCache, "d2", "/vol2", 12*gb, 100*gb)
	// The warned disk is 88% used, the other 50%. The meter may start from the
	// scratch project's disk, so 88% is past every level edge plus its margin.
	r.DiskMeterWhen(t, "high at level 4", diskIs(wire.HealthStateHigh, 4))
}

// Seam: no disk read yet is unknown and waiting.
func TestDiskHealth_NoDiskIsWaiting(t *testing.T) {
	t.Parallel()
	r := servertest.StartDiskRig(t, nil, 20*gb, func(c *server.Config) {
		c.FreeSpace = func(string) (wire.DiskReading, error) { return wire.DiskReading{}, errors.New("no disk") }
	})
	time.Sleep(quiet) // negative wait: failed readings never leave the waiting state
	got := r.DiskMeterWhen(t, "waiting", func(m wire.MeterHealth) bool { return true })
	if got.State != wire.HealthStateUnknown || got.Reason != wire.HealthReasonWaiting {
		t.Fatalf("disk = %+v, want unknown, waiting", got)
	}
}

// Seam: the disk status route lists total bytes for each disk.
func TestDiskStatus_ListsTotalBytes(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := servertest.StartDiskRig(t, map[string]string{"proj": repo}, 20*gb)
	r.Disks.PutSized(repo, "d1", "/vol1", 30*gb, 100*gb)
	r.DiskMeterWhen(t, "a disk", diskIs(wire.HealthStateNormal, 3))
	rep, err := r.H.Client.Admin.DiskStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d := servertest.DiskByID(rep, "d1"); d.TotalBytes != 100*gb {
		t.Fatalf("disk = %+v, want total 100G", d)
	}
}

// Seam: the health event fires only on a disk level or state change.
func TestDiskHealth_EventOnlyOnLevelOrStateChange(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := servertest.StartDiskRig(t, map[string]string{"proj": repo}, 20*gb)
	r.Disks.PutSized(repo, "d1", "/vol1", 30*gb, 100*gb)
	r.DiskMeterWhen(t, "normal at level 3", diskIs(wire.HealthStateNormal, 3))
	snap, err := r.H.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := r.H.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	r.Disks.SetFree(repo, 28*gb) // 72% used: same level, still normal
	r.WaitReading(t, "d1", 28*gb)
	r.Disks.SetFree(repo, 15*gb) // 85% used and warned
	r.DiskMeterWhen(t, "high at level 4", diskIs(wire.HealthStateHigh, 4))
	if n := servertest.HealthEvents(ch); n != 1 {
		t.Fatalf("health events = %d, want 1", n)
	}
}
