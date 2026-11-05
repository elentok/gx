package lifecycle_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/migratefail"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

func TestMigrationBlock_FailedProjectIsNotClaimedAndOthersAre(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}, "02": {}},
		func(_ *schedFixture, p *servertest.Turn, id string) { commitWork(t, p, id) })
	servertest.WriteTicketWith(t, f.Store, "other", schedEpic, "01", "t", servertest.TicketOpts{})
	servertest.SetProjectRepoNoAutoMerge(t, f.Store, "other", testutil.TempRepo(t))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// The start-up migration run clears a failure of a project it migrates, so
	// record ours after it ends.
	if !servertest.Poll(10*time.Second, func() bool {
		snap, err := f.H.Client.Snapshot(ctx)
		return err == nil && !snap.Migrating
	}) {
		t.Fatal("start-up migration did not end")
	}
	if err := migratefail.Record(f.H.StateDir, "proj", "boom"); err != nil {
		t.Fatal(err)
	}

	for _, addr := range []string{"proj:" + schedEpic + "/01", "other:" + schedEpic + "/01"} {
		// The index may not have seen a ticket written after start yet.
		var res wire.QueueResult
		var err error
		if !servertest.Poll(10*time.Second, func() bool {
			res, err = f.H.Client.Queue.Add(ctx, addr, "claude")
			return err == nil && !res.Refused
		}) {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	if !servertest.Poll(10*time.Second, func() bool {
		return strings.Contains(servertest.ReadFile(t, otherFile(t, f.Store)), "status: done")
	}) {
		t.Fatal("the other project's ticket was not claimed")
	}
	if body := servertest.ReadFile(t, f.TicketFile(t, "01")); !strings.Contains(body, "status: open") {
		t.Fatalf("blocked project's ticket was claimed:\n%s", body)
	}

	snap, err := f.H.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var row wire.PendingRow
	for _, r := range snap.Pending {
		if r.Address == "proj:"+schedEpic+"/01" {
			row = r
		}
	}
	if row.Verdict != wire.VerdictMigrationBlocked || row.Reason != "boom" {
		t.Fatalf("queue row = %+v, want %q with reason boom", row, wire.VerdictMigrationBlocked)
	}

	// A person can still act on a ticket of the blocked project.
	if res, err := f.H.Client.Tickets.Pause(ctx, "proj:"+schedEpic+"/02", "later"); err != nil || res.Refused {
		t.Fatalf("pause on a blocked project: %+v, %v", res, err)
	}

	if err := migratefail.Clear(f.H.StateDir, "proj"); err != nil {
		t.Fatal(err)
	}
	if !servertest.Poll(10*time.Second, func() bool {
		return !strings.Contains(servertest.ReadFile(t, f.TicketFile(t, "01")), "status: open")
	}) {
		t.Fatal("the ticket was not claimed after the failure cleared")
	}
}

func otherFile(t *testing.T, store string) string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(store, "other", schedEpic, "issues", "01-*.md"))
	if err != nil || len(m) != 1 {
		t.Fatalf("other ticket files = %v, %v", m, err)
	}
	return m[0]
}
