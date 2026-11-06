package servertest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
)

const (
	// GB is the unit of the disk sizes in disk tests.
	GB = 1_000_000_000
	// DiskInterval is the watcher's poll interval in a DiskRig.
	DiskInterval = 10 * time.Millisecond
	// FlushInterval stands in for the chat batcher's 6s tick in tests that
	// count sends but never assert two notices share one batch.
	FlushInterval = 100 * time.Millisecond
)

// DiskRig is a server whose projects are watched by a fake free-space source.
type DiskRig struct {
	H     *Harness
	Disks *FakeDisks
	Store string
	// WarnBodies are the chat bodies of the warn notices sent so far.
	WarnBodies func() []string
}

// Notice is how many warn notices were sent so far.
func (r *DiskRig) Notice() int { return len(r.WarnBodies()) }

// StartDiskRig starts a server whose projects (name -> repo path) are watched
// by a fake free-space source; warn is the warn threshold in bytes.
func StartDiskRig(t *testing.T, projects map[string]string, warn config.ByteSize, opts ...func(*server.Config)) *DiskRig {
	t.Helper()
	store := t.TempDir()
	for name, repo := range projects {
		if err := os.MkdirAll(filepath.Join(store, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := WriteProjectRepo(store, name, repo, ""); err != nil {
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
	rig := &DiskRig{Disks: &FakeDisks{}, Store: store}
	rig.WarnBodies = func() []string {
		mu.Lock()
		defer mu.Unlock()
		var out []string
		for _, b := range bodies {
			if strings.Contains(b, "Low disk space") {
				out = append(out, b)
			}
		}
		return out
	}
	all := append([]func(*server.Config){func(c *server.Config) {
		c.FreeSpace = rig.Disks.Read
		c.DiskPollInterval = DiskInterval
		c.DiskSpace = config.DiskSpaceConfig{WarnBelow: warn, CleanBelow: 10 * GB}
		c.Chat = notify.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: FlushInterval}
	}}, opts...)
	rig.H = StartWithStore(t, store, all...)
	return rig
}

// DisksWhen polls the snapshot until pred accepts its disk list.
func (r *DiskRig) DisksWhen(t *testing.T, what string, pred func([]wire.DiskStatus) bool) []wire.DiskStatus {
	t.Helper()
	var last []wire.DiskStatus
	WaitFor(t, 10*time.Second, "snapshot disks "+what, func() bool {
		snap, err := r.H.Client.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		last = snap.Disks
		return pred(last)
	})
	return last
}

// WaitNotices waits until n warn notices were sent.
func (r *DiskRig) WaitNotices(t *testing.T, n int) {
	t.Helper()
	WaitFor(t, 10*time.Second, fmt.Sprintf("%d warn notices", n), func() bool { return r.Notice() >= n })
}

// WaitReading waits until the watcher has read the disk id with free bytes, so
// a later check sees the effect of a SetFree. The status route lists every
// disk, warned or not.
func (r *DiskRig) WaitReading(t *testing.T, id string, free uint64) {
	t.Helper()
	WaitFor(t, 10*time.Second, fmt.Sprintf("a reading of %s at %d bytes", id, free), func() bool {
		rep, err := r.H.Client.Admin.DiskStatus(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return DiskByID(rep, id).FreeBytes == free
	})
}

// DiskByID is the disk with this id in rep; the zero value when it is not listed.
func DiskByID(rep wire.DiskReport, id string) wire.DiskStatus {
	for _, d := range rep.Disks {
		if d.ID == id {
			return d
		}
	}
	return wire.DiskStatus{}
}

// HealthEvents counts the health-changed events on stream until it is quiet.
func HealthEvents(ch <-chan wire.Event) int {
	n := 0
	for {
		select {
		case ev := <-ch:
			if ev.Type == wire.EventHealthChanged {
				n++
			}
		case <-time.After(150 * time.Millisecond):
			return n
		}
	}
}

// DiskMeterWhen polls the snapshot until pred accepts the disk meter.
func (r *DiskRig) DiskMeterWhen(t *testing.T, what string, pred func(wire.MeterHealth) bool) wire.MeterHealth {
	t.Helper()
	var last wire.MeterHealth
	if !Poll(10*time.Second, func() bool {
		snap, err := r.H.Client.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		last = snap.Health.Disk
		return pred(last)
	}) {
		t.Fatalf("snapshot disk meter = %+v, want %s", last, what)
	}
	return last
}

// Await is WaitFor with the 15s limit most tests use.
func Await(t *testing.T, ok func() bool) {
	t.Helper()
	WaitFor(t, 15*time.Second, "the condition", ok)
}
