// Package mergetest holds the merge fixture helpers shared by the merge
// package's tests and the mergeflows end-to-end tests.
package mergetest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

// Fixture adds the merge-only helpers to the shared servertest.SchedFixture.
type Fixture struct {
	*servertest.SchedFixture
}

// PauseRoot runs the epic to its root pause, failing on an iteration-failed on
// the way, and returns the pause event.
func (f *Fixture) PauseRoot(t *testing.T) wire.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	for {
		select {
		case ev, open := <-evs:
			if !open {
				t.Fatal("event stream ended before root-paused")
			}
			switch ev.Type {
			case wire.EventIterationFailed:
				t.Fatalf("iteration-failed on a merge error: %+v", ev)
			case wire.EventRootPaused:
				return ev
			}
		case <-ctx.Done():
			t.Fatalf("no root-paused: %v", ctx.Err())
		}
	}
}

// AssertRootQueued checks the paused root was not dequeued.
func (f *Fixture) AssertRootQueued(t *testing.T) {
	t.Helper()
	q, err := f.H.Client.Queue.Items(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range q {
		if e.Address == "proj:"+servertest.SchedEpic+"/01" {
			return
		}
	}
	t.Errorf("root dequeued: %+v", q)
}

// EpicMD is the epic's ticket.md, read after the root settled.
func (f *Fixture) EpicMD(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.Store, "proj", servertest.SchedEpic, "ticket.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// AssertStamps checks completed_at is stamped and merged_at is stamped only when wantMerged.
func (f *Fixture) AssertStamps(t *testing.T, wantMerged bool) {
	t.Helper()
	md := f.EpicMD(t)
	if !strings.Contains(md, "completed_at:") {
		t.Errorf("epic ticket.md has no completed_at:\n%s", md)
	}
	if got := strings.Contains(md, "merged_at:"); got != wantMerged {
		t.Errorf("epic ticket.md has merged_at = %v, want %v:\n%s", got, wantMerged, md)
	}
	if !strings.Contains(md, "status:") {
		t.Errorf("epic ticket.md lost its other keys:\n%s", md)
	}
}

// FinishCommitless is the agent's turn for a ticket that needs no commit.
func (f *Fixture) FinishCommitless(t *testing.T, id string) {
	t.Helper()
	path := f.TicketFile(t, id)
	body := strings.Replace(servertest.ReadFile(t, path), "---\n", "---\ncommitless: true\niteration_status: finished\n", 1)
	testutil.WriteFile(t, filepath.Dir(path), filepath.Base(path), body)
}
