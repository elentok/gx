package projects_test

import (
	"context"
	"github.com/elentok/gx/server/wire"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/migratefail"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// Seam: a change to the record file while the server runs shows in what it reports.
func TestMigrationFailures_FileChangeIsSeenWithoutRestart(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	proj := filepath.Join(store, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "project.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.MigrationFailurePollInterval = 10 * time.Millisecond })
	// A passing start clears the record, so seed it after the start-up run ends.
	<-h.Server.MigrationsDone()

	if got := h.Server.MigrationFailures(); len(got) != 0 {
		t.Fatalf("failures at start: %v", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if err := migratefail.Record(h.StateDir, "proj", "boom"); err != nil {
		t.Fatal(err)
	}
	if err := migratefail.Record(h.StateDir, "removed", "ignored"); err != nil {
		t.Fatal(err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventMigrationFailuresChanged, "")
	servertest.WaitFor(t, 10*time.Second, "failure reported", func() bool {
		got := h.Server.MigrationFailures()
		return len(got) == 1 && got[0].Project == "proj" && got[0].Error == "boom"
	})

	if err := migratefail.Clear(h.StateDir, "proj"); err != nil {
		t.Fatal(err)
	}
	servertest.WaitFor(t, 10*time.Second, "failure cleared", func() bool {
		return len(h.Server.MigrationFailures()) == 0
	})
}

// Seam: the live snapshot lists a recorded failure with its retry command, and
// drops it once the record is cleared.
func TestLiveSnapshot_ListsAndDropsMigrationFailure(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	proj := filepath.Join(store, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "project.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)
	// A passing start clears the record, so seed it after the start-up run ends.
	<-h.Server.MigrationsDone()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := migratefail.Record(h.StateDir, "proj", "boom"); err != nil {
		t.Fatal(err)
	}
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.MigrationFailures) != 1 {
		t.Fatalf("failures = %v, want one", snap.MigrationFailures)
	}
	f := snap.MigrationFailures[0]
	if f.Project != "proj" || f.Error != "boom" || f.RetryCommand != "gx tickets migrate '"+proj+"'" {
		t.Errorf("failure = %+v", f)
	}

	if err := migratefail.Clear(h.StateDir, "proj"); err != nil {
		t.Fatal(err)
	}
	snap, err = h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.MigrationFailures) != 0 {
		t.Errorf("failures after clear = %v", snap.MigrationFailures)
	}
}

// Seam: the instance id is stable within one start and differs across starts.
func TestLiveSnapshot_InstanceIDIsPerStart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := servertest.StartWithStore(t, t.TempDir())
	b := servertest.StartWithStore(t, t.TempDir())
	first, err := a.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := a.Client.Snapshot(ctx)
	other, _ := b.Client.Snapshot(ctx)
	if first.Instance == "" || first.Instance != again.Instance {
		t.Errorf("instance not stable: %q then %q", first.Instance, again.Instance)
	}
	if first.Instance == other.Instance {
		t.Errorf("two starts share instance %q", first.Instance)
	}
}
