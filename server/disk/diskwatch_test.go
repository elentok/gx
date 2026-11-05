package disk_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

const (
	gb           = servertest.GB
	diskInterval = servertest.DiskInterval
)

// quiet is how long a negative check waits for something that must not
// happen: several watcher readings and a chat flush. Only for checks with no
// later positive event to wait on.
const quiet = 15*diskInterval + 2*servertest.FlushInterval

func oneDisk(d []wire.DiskStatus) bool { return len(d) == 1 }
func noDisks(d []wire.DiskStatus) bool { return len(d) == 0 }

// Seam: the watcher reads with no queued root.
func TestDiskWatch_LowDiskShowsInSnapshotWithNoQueue(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := servertest.StartDiskRig(t, map[string]string{"proj": repo}, 20*gb)
	r.Disks.Put(repo, "d1", "/vol1", 5*gb)
	got := r.DisksWhen(t, "one warned disk", oneDisk)[0]
	if got.ID != "d1" || got.Name != "/vol1" || got.FreeBytes != 5*gb || !slices.Equal(got.Projects, []string{"proj"}) {
		t.Errorf("disk = %+v", got)
	}
}

// Seam: the Go cache disk is watched for every project.
func TestDiskWatch_GoCacheDiskAloneNamesTheProject(t *testing.T) {
	t.Parallel()
	repo, goCache := t.TempDir(), t.TempDir()
	r := servertest.StartDiskRig(t, map[string]string{"proj": repo}, 20*gb, func(c *server.Config) {
		c.GoCacheDir = func() (string, error) { return goCache, nil }
	})
	r.Disks.Put(goCache, "gc", "/gocache", 1*gb)
	got := r.DisksWhen(t, "the Go cache disk", oneDisk)[0]
	// The built-in scratch project has a repo too, so it watches the cache as well.
	if got.ID != "gc" || !slices.Contains(got.Projects, "proj") {
		t.Errorf("disk = %+v", got)
	}
}

// Seam: projects on one disk share one entry and one notice.
func TestDiskWatch_TwoProjectsOnOneDiskShareEntryAndNotice(t *testing.T) {
	t.Parallel()
	a, b := t.TempDir(), t.TempDir()
	r := servertest.StartDiskRig(t, map[string]string{"a": a, "b": b}, 20*gb)
	r.Disks.Put(a, "d1", "/vol1", 5*gb)
	r.Disks.Put(b, "d1", "/vol1", 5*gb)
	got := r.DisksWhen(t, "one entry for both projects", func(d []wire.DiskStatus) bool {
		return len(d) == 1 && len(d[0].Projects) == 2
	})[0]
	if !slices.Equal(got.Projects, []string{"a", "b"}) {
		t.Errorf("projects = %v", got.Projects)
	}
	r.WaitNotices(t, 1)
	time.Sleep(quiet) // negative wait: no second notice follows
	if n := r.Notice(); n != 1 {
		t.Errorf("warn notices = %d, want 1", n)
	}
}

// Seam: a crossing sends one notice naming the disk, its free space and the threshold.
func TestDiskWatch_CrossingSendsNoticeWithDiskAndFreeSpace(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := servertest.StartDiskRig(t, map[string]string{"proj": repo}, 20*gb)
	r.Disks.Put(repo, "d1", "/vol1", 5*gb)
	r.WaitNotices(t, 1)
	bodies := r.WarnBodies()
	if len(bodies) != 1 {
		t.Fatalf("warn bodies = %q, want one", bodies)
	}
	for _, want := range []string{"/vol1", "5.0 GB", "20 GB"} {
		if !strings.Contains(bodies[0], want) {
			t.Errorf("notice %q lacks %q", bodies[0], want)
		}
	}
}

// Seam: hovering at the threshold sends one notice; a 2G climb re-arms it.
func TestDiskWatch_HysteresisSendsOneNoticePerDrop(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := servertest.StartDiskRig(t, map[string]string{"proj": repo}, 20*gb)
	r.Disks.Put(repo, "d1", "/vol1", 19*gb)
	r.DisksWhen(t, "warned", oneDisk)
	for range 3 {
		r.Disks.SetFree(repo, 21*gb)
		r.WaitReading(t, "d1", 21*gb)
		r.Disks.SetFree(repo, 19*gb)
		r.WaitReading(t, "d1", 19*gb)
	}
	time.Sleep(quiet) // negative wait: a re-armed notice would arrive within a flush
	if n := r.Notice(); n != 1 {
		t.Fatalf("warn notices after hovering = %d, want 1", n)
	}
	r.Disks.SetFree(repo, 23*gb)
	r.DisksWhen(t, "cleared after the climb", noDisks)
	r.Disks.SetFree(repo, 5*gb)
	r.DisksWhen(t, "warned again", oneDisk)
	r.WaitNotices(t, 2)
}

// Seam: a zero warn threshold never marks a disk.
func TestDiskWatch_ZeroWarnThresholdSendsNothing(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := servertest.StartDiskRig(t, map[string]string{"proj": repo}, 0, func(c *server.Config) { c.DiskSpace.CleanBelow = 0 })
	r.Disks.Put(repo, "d1", "/vol1", 1)
	time.Sleep(quiet) // negative wait: no disk is ever marked
	if d := r.DisksWhen(t, "no disks", noDisks); len(d) != 0 || r.Notice() != 0 {
		t.Errorf("disks = %+v, notices = %d, want none", d, r.Notice())
	}
}

// Seam: a reload applies the new threshold at the next reading.
func TestDiskWatch_ReloadAppliesAtNextReading(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	r := servertest.StartDiskRig(t, map[string]string{"proj": repo}, 20*gb, servertest.WithConfigFile(cfgPath))
	r.Disks.Put(repo, "d1", "/vol1", 30*gb)
	r.WaitReading(t, "d1", 30*gb)
	r.DisksWhen(t, "no disks under 20G", noDisks)

	servertest.WriteConfig(t, cfgPath, r.Store, map[string]any{"disk-space": map[string]any{"warn-below": "50G", "clean-below": "10G"}})
	if res, err := r.H.Client.Admin.ReloadConfig(context.Background()); err != nil || res.Refused {
		t.Fatalf("reload: %+v, %v", res, err)
	}
	r.DisksWhen(t, "warned under 50G", oneDisk)
}
