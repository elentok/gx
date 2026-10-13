package server_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// parkRoot runs the epic to its root park, failing on an iteration-failed on
// the way, and returns the park event.
func (f *schedFixture) parkRoot(t *testing.T) server.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	for {
		select {
		case ev, open := <-evs:
			if !open {
				t.Fatal("event stream ended before root-parked")
			}
			switch ev.Type {
			case server.EventIterationFailed:
				t.Fatalf("iteration-failed on a land error: %+v", ev)
			case server.EventRootParked:
				return ev
			}
		case <-ctx.Done():
			t.Fatalf("no root-parked: %v", ctx.Err())
		}
	}
}

// assertRootQueued checks the parked root was not dequeued.
func (f *schedFixture) assertRootQueued(t *testing.T) {
	t.Helper()
	q, err := f.h.Client.Queue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range q {
		if e.Address == "proj:"+schedEpic+"/01" {
			return
		}
	}
	t.Errorf("root dequeued: %+v", q)
}

// commitFiles is the agent's turn committing every name as a file.
func commitFiles(t *testing.T, p servertest.Prompt, names ...string) {
	t.Helper()
	for _, n := range names {
		testutil.WriteFile(t, p.Cwd, n, n)
	}
	testutil.CommitAll(t, p.Cwd, "work")
}

func TestLandError_DirtyTrunkParksTheRootWithACappedFileList(t *testing.T) {
	chat, count := chatRecorder(t)
	files := []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt", "f.txt", "g.txt"}
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, _ string) {
		commitFiles(t, p, files...)
		// Untracked files in main's checkout that the land would overwrite.
		for _, n := range files {
			testutil.WriteFile(t, f.repo, n, "local "+n)
		}
	}, chat)
	main := f.git(t, "rev-parse", "main")

	ev := f.parkRoot(t)

	if ev.Kind != string(events.DirtyTrunk) {
		t.Errorf("kind = %q, want %q", ev.Kind, events.DirtyTrunk)
	}
	want := fmt.Sprintf("trunk dirty: main checked out at %s has local changes in a.txt, b.txt, c.txt, d.txt, e.txt +2 more; "+
		"commit or discard them, then run gx server epics merge proj:%s", f.repo, schedEpic)
	if ev.Message != want {
		t.Errorf("reason =\n%q\nwant\n%q", ev.Message, want)
	}
	if got := f.git(t, "rev-parse", "main"); got != main {
		t.Errorf("main moved to %s", got)
	}
	f.assertRootQueued(t)
	f.assertStamps(t, false)
	if got := waitForChat(count, "trunk dirty"); got != 1 {
		t.Errorf("park sent %d times, want once", got)
	}
	if got := count(epicNotMergedText) + count(epicMergedText); got != 0 {
		t.Errorf("epic notice sent %d times, want none", got)
	}
}

func TestLandError_NonOverlappingLocalChangeDoesNotBlockTheLand(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
		testutil.WriteFile(t, f.repo, "unrelated.txt", "local")
	})
	f.runEpic(t, server.EventRootCompleted)

	if got := f.git(t, "rev-parse", "main"); got != f.git(t, "rev-parse", schedEpic) {
		t.Errorf("main at %s, want the feature branch tip", got)
	}
}

func TestLandError_OtherGitFailureParksLandFailed(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
		// A stale index lock fails the merge without any local change.
		if err := os.WriteFile(filepath.Join(f.repo, ".git", "index.lock"), nil, 0o644); err != nil {
			t.Error(err)
		}
	})

	ev := f.parkRoot(t)

	if ev.Kind != string(events.LandError) {
		t.Errorf("kind = %q, want %q", ev.Kind, events.LandError)
	}
	if !strings.HasPrefix(ev.Message, "land failed: ") || !strings.Contains(ev.Message, "index.lock") ||
		!strings.HasSuffix(ev.Message, "; fix it, then run gx server epics merge proj:"+schedEpic) {
		t.Errorf("reason = %q", ev.Message)
	}
	f.assertRootQueued(t)
	f.assertStamps(t, false)
}
