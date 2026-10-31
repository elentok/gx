package server_test

import (
	"context"
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
	servertest.WaitForEvent(ctx, t, evs, server.EventMigrationFailuresChanged, "")
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
