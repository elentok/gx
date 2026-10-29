package server_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

const (
	gb           = 1_000_000_000
	diskInterval = 10 * time.Millisecond
)

type diskRig struct {
	h     *servertest.Harness
	disks *servertest.FakeDisks
	store string
	// warnBodies are the chat bodies of the warn notices sent so far.
	warnBodies func() []string
}

func (r *diskRig) notice() int { return len(r.warnBodies()) }

// startDiskRig starts a server whose projects (name -> repo path) are watched
// by a fake free-space source; warn is the warn threshold in bytes.
func startDiskRig(t *testing.T, projects map[string]string, warn config.ByteSize, opts ...func(*server.Config)) *diskRig {
	t.Helper()
	store := t.TempDir()
	for name, repo := range projects {
		if err := os.MkdirAll(filepath.Join(store, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := servertest.WriteProjectRepo(store, name, repo, ""); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	var bodies []string
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
	}))
	t.Cleanup(chat.Close)
	rig := &diskRig{disks: &servertest.FakeDisks{}, store: store}
	rig.warnBodies = func() []string {
		mu.Lock()
		defer mu.Unlock()
		var out []string
		for _, b := range bodies {
			if strings.Contains(b, "low disk space") {
				out = append(out, b)
			}
		}
		return out
	}
	all := append([]func(*server.Config){func(c *server.Config) {
		c.FreeSpace = rig.disks.Read
		c.DiskPollInterval = diskInterval
		c.DiskSpace = config.DiskSpaceConfig{WarnBelow: warn, CleanBelow: 10 * gb}
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: testFlushInterval}
	}}, opts...)
	rig.h = servertest.StartWithStore(t, store, all...)
	return rig
}

// disksWhen polls the snapshot until pred accepts its disk list.
func (r *diskRig) disksWhen(t *testing.T, what string, pred func([]server.DiskStatus) bool) []server.DiskStatus {
	t.Helper()
	var last []server.DiskStatus
	servertest.WaitFor(t, 10*time.Second, "snapshot disks "+what, func() bool {
		snap, err := r.h.Client.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		last = snap.Disks
		return pred(last)
	})
	return last
}

func (r *diskRig) waitNotices(t *testing.T, n int) {
	t.Helper()
	servertest.WaitFor(t, 10*time.Second, fmt.Sprintf("%d warn notices", n), func() bool { return r.notice() >= n })
}

// waitReading waits until the watcher has read the disk id with free bytes, so
// a later check sees the effect of a SetFree. The status route lists every
// disk, warned or not.
func (r *diskRig) waitReading(t *testing.T, id string, free uint64) {
	t.Helper()
	servertest.WaitFor(t, 10*time.Second, fmt.Sprintf("a reading of %s at %d bytes", id, free), func() bool {
		rep, err := r.h.Client.DiskStatus(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return diskByID(rep, id).FreeBytes == free
	})
}

// quiet is how long a negative check waits for something that must not
// happen: several watcher readings and a chat flush. Only for checks with no
// later positive event to wait on.
const quiet = 15*diskInterval + 2*testFlushInterval

func oneDisk(d []server.DiskStatus) bool { return len(d) == 1 }
func noDisks(d []server.DiskStatus) bool { return len(d) == 0 }

// Seam: the watcher reads with no queued root.
func TestDiskWatch_LowDiskShowsInSnapshotWithNoQueue(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := startDiskRig(t, map[string]string{"proj": repo}, 20*gb)
	r.disks.Put(repo, "d1", "/vol1", 5*gb)
	got := r.disksWhen(t, "one warned disk", oneDisk)[0]
	if got.ID != "d1" || got.Name != "/vol1" || got.FreeBytes != 5*gb || !slices.Equal(got.Projects, []string{"proj"}) {
		t.Errorf("disk = %+v", got)
	}
}

// Seam: the Go cache disk is watched for every project.
func TestDiskWatch_GoCacheDiskAloneNamesTheProject(t *testing.T) {
	t.Parallel()
	repo, goCache := t.TempDir(), t.TempDir()
	r := startDiskRig(t, map[string]string{"proj": repo}, 20*gb, func(c *server.Config) {
		c.GoCacheDir = func() (string, error) { return goCache, nil }
	})
	r.disks.Put(goCache, "gc", "/gocache", 1*gb)
	got := r.disksWhen(t, "the Go cache disk", oneDisk)[0]
	// The built-in scratch project has a repo too, so it watches the cache as well.
	if got.ID != "gc" || !slices.Contains(got.Projects, "proj") {
		t.Errorf("disk = %+v", got)
	}
}

// Seam: projects on one disk share one entry and one notice.
func TestDiskWatch_TwoProjectsOnOneDiskShareEntryAndNotice(t *testing.T) {
	t.Parallel()
	a, b := t.TempDir(), t.TempDir()
	r := startDiskRig(t, map[string]string{"a": a, "b": b}, 20*gb)
	r.disks.Put(a, "d1", "/vol1", 5*gb)
	r.disks.Put(b, "d1", "/vol1", 5*gb)
	got := r.disksWhen(t, "one entry for both projects", func(d []server.DiskStatus) bool {
		return len(d) == 1 && len(d[0].Projects) == 2
	})[0]
	if !slices.Equal(got.Projects, []string{"a", "b"}) {
		t.Errorf("projects = %v", got.Projects)
	}
	r.waitNotices(t, 1)
	time.Sleep(quiet) // negative wait: no second notice follows
	if n := r.notice(); n != 1 {
		t.Errorf("warn notices = %d, want 1", n)
	}
}

// Seam: a crossing sends one notice naming the disk, its free space and the threshold.
func TestDiskWatch_CrossingSendsNoticeWithDiskAndFreeSpace(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := startDiskRig(t, map[string]string{"proj": repo}, 20*gb)
	r.disks.Put(repo, "d1", "/vol1", 5*gb)
	r.waitNotices(t, 1)
	bodies := r.warnBodies()
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
	r := startDiskRig(t, map[string]string{"proj": repo}, 20*gb)
	r.disks.Put(repo, "d1", "/vol1", 19*gb)
	r.disksWhen(t, "warned", oneDisk)
	for range 3 {
		r.disks.SetFree(repo, 21*gb)
		r.waitReading(t, "d1", 21*gb)
		r.disks.SetFree(repo, 19*gb)
		r.waitReading(t, "d1", 19*gb)
	}
	time.Sleep(quiet) // negative wait: a re-armed notice would arrive within a flush
	if n := r.notice(); n != 1 {
		t.Fatalf("warn notices after hovering = %d, want 1", n)
	}
	r.disks.SetFree(repo, 23*gb)
	r.disksWhen(t, "cleared after the climb", noDisks)
	r.disks.SetFree(repo, 5*gb)
	r.disksWhen(t, "warned again", oneDisk)
	r.waitNotices(t, 2)
}

// Seam: a zero warn threshold never marks a disk.
func TestDiskWatch_ZeroWarnThresholdSendsNothing(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	r := startDiskRig(t, map[string]string{"proj": repo}, 0, func(c *server.Config) { c.DiskSpace.CleanBelow = 0 })
	r.disks.Put(repo, "d1", "/vol1", 1)
	time.Sleep(quiet) // negative wait: no disk is ever marked
	if d := r.disksWhen(t, "no disks", noDisks); len(d) != 0 || r.notice() != 0 {
		t.Errorf("disks = %+v, notices = %d, want none", d, r.notice())
	}
}

// Seam: a reload applies the new threshold at the next reading.
func TestDiskWatch_ReloadAppliesAtNextReading(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	r := startDiskRig(t, map[string]string{"proj": repo}, 20*gb, servertest.WithConfigFile(cfgPath))
	r.disks.Put(repo, "d1", "/vol1", 30*gb)
	r.waitReading(t, "d1", 30*gb)
	r.disksWhen(t, "no disks under 20G", noDisks)

	servertest.WriteConfig(t, cfgPath, r.store, map[string]any{"disk-space": map[string]any{"warn-below": "50G", "clean-below": "10G"}})
	if res, err := r.h.Client.ReloadConfig(context.Background()); err != nil || res.Refused {
		t.Fatalf("reload: %+v, %v", res, err)
	}
	r.disksWhen(t, "warned under 50G", oneDisk)
}
