package server_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
)

// pastCLINotice is longer than the 1 s after which the CLI prints "waiting for
// the migration of <project>": a write has no time limit of its own.
const pastCLINotice = 1500 * time.Millisecond

type writeResult struct {
	res server.QueueResult
	err error
}

// assertWaitsForMigration starts write while the migration is held, checks it
// stays pending past the CLI notice, releases the migration and returns what
// the write answered.
func assertWaitsForMigration[T any](t *testing.T, release func(), write func() T) T {
	t.Helper()
	done := make(chan T, 1)
	go func() { done <- write() }()
	select {
	case <-done:
		t.Fatal("write returned while its project migrated")
	case <-time.After(pastCLINotice):
	}
	release()
	select {
	case got := <-done:
		return got
	case <-time.After(10 * time.Second):
		t.Fatal("write still waiting after the migration ended")
	}
	panic("unreachable")
}

// Story 35: a write on the agent-output path (a permission answer logged on the
// ticket) waits while its project migrates, with no time limit, and lands after.
func TestMigrate_AgentOutputWriteWaitsForTheMigrationThenLands(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	fake := runnerfake.NewRunner()
	const label = "epic-a-iter-01"
	if _, err := fake.Start(agentrunner.StartOptions{Label: label, Epic: "epic-a", Cwd: t.TempDir(), Kind: "claude"}); err != nil {
		t.Fatal(err)
	}
	fake.SetState(label, agentrunner.StateBlocked, "permission to use Bash")
	opt, entered, release := heldMigration(t)
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), opt)
	<-entered
	ticket := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")

	got := assertWaitsForMigration(t, release, func() writeResult {
		res, err := h.Client.AgentAnswer(context.Background(), "proj:epic-a/01", "allow", "")
		return writeResult{res, err}
	})
	if got.err != nil || got.res.Refused {
		t.Fatalf("answer = %+v, %v", got.res, got.err)
	}
	events, err := tickets.ReadLog(ticket)
	if err != nil {
		t.Fatal(err)
	}
	var logged bool
	for _, ev := range events {
		logged = logged || ev.Type == tickets.EventPausePermission
	}
	if !logged {
		t.Errorf("permission answer not in the ticket log after the migration: %+v", events)
	}
}

// Story 35: a write on the land path (the merge stamp on the epic) waits while
// its project migrates, with no time limit, and lands after.
func TestMigrate_LandWriteWaitsForTheMigrationThenLands(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "research", "01", "t", servertest.TicketOpts{Status: "done", Type: "research"})
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	opt, entered, release := heldMigration(t)
	h := servertest.StartWithStore(t, store, opt)
	<-entered

	got := assertWaitsForMigration(t, release, func() writeResult {
		res, err := h.Client.EpicMerge(context.Background(), "proj:research")
		return writeResult{res, err}
	})
	if got.err != nil || got.res.Refused || !got.res.Merged {
		t.Fatalf("merge = %+v, %v, want merged", got.res, got.err)
	}
	if md := tickets.LoadEpicTicketMD(filepath.Join(store, "proj", "research")); md.MergedAt.IsZero() {
		t.Error("epic has no merged_at after the migration")
	}
}
