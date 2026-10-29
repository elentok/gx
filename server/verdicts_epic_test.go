package server_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// startCapHeldEpics starts a server whose project cap is full from a run in
// epic-a, with epic-b (tickets 01 and 02) free to be queued.
func startCapHeldEpics(t *testing.T) (*servertest.Harness, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "02", "second", "")
	body := fmt.Sprintf(`{"name":"proj","repo":%q,"max-agents":1}`, repo)
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
		c.MaxAgents = 1
	})
	registerLaunch(h)
	h.Server.PutRun("proj:epic-a", server.Run{Address: "proj:epic-a/00"})
	return h, repo
}

func TestPending_WholeEpicEntryGivesOneRowForItsFrontierTicket(t *testing.T) {
	h, _ := startCapHeldEpics(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-b", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Pending) != 1 || snap.Pending[0].Address != "proj:epic-b/01" || snap.Pending[0].Verdict != server.VerdictConcurrencyCap {
		t.Fatalf("pending = %+v, want one %s row for proj:epic-b/01", snap.Pending, server.VerdictConcurrencyCap)
	}
}

func TestPending_WholeEpicEntrySkipsAFrontierTicketThatIsAlreadyRunning(t *testing.T) {
	h, _ := startCapHeldEpics(t)
	h.Server.PutRun("proj:epic-b/01", server.Run{Address: "proj:epic-b/01"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-b", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range snap.Pending {
		if row.Address == "proj:epic-b/01" {
			t.Fatalf("pending = %+v, want no row for the running proj:epic-b/01", snap.Pending)
		}
	}
}

func TestPending_TicketQueuedAloneAndItsWholeEpicGiveOneRow(t *testing.T) {
	h, _ := startCapHeldEpics(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, addr := range []string{"proj:epic-b", "proj:epic-b/01"} {
		if res, err := h.Client.QueueAdd(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Pending) != 1 || snap.Pending[0].Address != "proj:epic-b/01" {
		t.Fatalf("pending = %+v, want one row for proj:epic-b/01", snap.Pending)
	}
}

func TestPending_WholeEpicRowStreamsAVerdictChange(t *testing.T) {
	h, repo := startCapHeldEpics(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-b", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	// A launch would move the epic's frontier to the next ticket, so flip the
	// verdict with a missing repo instead, which never claims.
	// The first poll only sets the baseline, so keep flipping until a change
	// lands after it.
	gone := repo + ".gone"
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		from, to := repo, gone
		for {
			select {
			case <-stop:
				return
			case <-time.After(100 * time.Millisecond):
				if os.Rename(from, to) == nil {
					from, to = to, from
				}
			}
		}
	}()
	for ev := range evs {
		if ev.Type == server.EventExplainVerdictChange && ev.Address == "proj:epic-b/01" {
			return
		}
	}
	t.Fatal("no verdict-change event for the epic entry's row")
}
