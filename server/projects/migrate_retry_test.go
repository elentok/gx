package projects_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/migratefail"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

func failedRecords(t *testing.T, h *servertest.Harness) []migratefail.Failure {
	t.Helper()
	fails, err := migratefail.Read(h.StateDir, []string{"proj"})
	if err != nil {
		t.Fatal(err)
	}
	return fails
}

// Seam 1: a retry runs the pending step, keeps the record while it fails and
// clears it once it passes.
func TestMigrate_RetryClearsTheRecordOnPass(t *testing.T) {
	t.Parallel()
	store := migrateStore(t, "proj")
	var broken atomic.Bool
	broken.Store(true)
	reg := &migrations.Registry{}
	reg.Register(migrations.Step{Name: "flaky", Run: func(env migrations.Env) ([]tickets.MigrationChange, error) {
		if filepath.Base(env.Root) == "proj" && broken.Load() {
			return nil, errors.New("exploded")
		}
		return nil, nil
	}})
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Migrations = reg })
	waitMigrations(t, h)
	if len(failedRecords(t, h)) != 1 {
		t.Fatalf("want one saved failure after start, got %v", failedRecords(t, h))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	retry := func() {
		t.Helper()
		res, err := h.Client.RetryMigration(ctx, wire.RetryMigrationRequest{Project: "proj"})
		if err != nil || !res.Started {
			t.Fatalf("retry = %+v, %v", res, err)
		}
	}
	retry()
	waitFor(t, func() bool { return !h.Server.MigrationRunning("proj") })
	if len(failedRecords(t, h)) != 1 {
		t.Errorf("a failing retry must keep the record, got %v", failedRecords(t, h))
	}

	broken.Store(false)
	retry()
	waitFor(t, func() bool { return len(failedRecords(t, h)) == 0 })
	if got := appliedOf(t, store, "proj"); got != "flaky\n" {
		t.Errorf("applied = %q", got)
	}
}

// Seam 1: a retry during a held run of that project is refused; an unknown
// project is refused too.
func TestMigrate_RetryRefusedWhileRunningOrUnknown(t *testing.T) {
	t.Parallel()
	store := migrateStore(t, "proj")
	entered := make(chan struct{})
	release := make(chan struct{})
	var once, relOnce sync.Once
	reg := &migrations.Registry{}
	reg.Register(migrations.Step{Name: "hold", Run: func(env migrations.Env) ([]tickets.MigrationChange, error) {
		if filepath.Base(env.Root) == "proj" {
			once.Do(func() { close(entered) })
			<-release
		}
		return nil, nil
	}})
	t.Cleanup(func() { relOnce.Do(func() { close(release) }) })
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Migrations = reg })
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.RetryMigration(ctx, wire.RetryMigrationRequest{Project: "proj"})
	if err != nil || !res.Refused || res.Reason != wire.ReasonRetryRunning {
		t.Errorf("during a run = %+v, %v", res, err)
	}
	res, err = h.Client.RetryMigration(ctx, wire.RetryMigrationRequest{Project: "nope"})
	if err != nil || !res.Refused || res.Reason != wire.ReasonRetryUnknownProject {
		t.Errorf("unknown = %+v, %v", res, err)
	}
	relOnce.Do(func() { close(release) })
	waitMigrations(t, h)
}
