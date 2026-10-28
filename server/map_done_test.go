package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// handMergeEpic hand-merges the complete epic blocker into a temp repo while a
// map named mapName sits in the store, and returns a loader for the map once
// the server stamped the blocker's merged_at.
func handMergeEpic(t *testing.T, blocker, mapName string) func() tickets.Epic {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	commitOnBranch(t, repo, blocker)
	writeCompleteEpic(t, store, blocker)
	if err := os.MkdirAll(filepath.Join(store, "proj", mapName), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, filepath.Join(store, "proj", mapName), "ticket.md", "---\nkind: map\nstatus: open\n---\n# Map\n")
	servertest.WriteTicketWith(t, store, "proj", "dep", "01", "d", servertest.TicketOpts{BlockedBy: []string{blocker}})
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	if res, err := h.Client.QueueAdd(context.Background(), "proj:dep/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	runGitIn(t, repo, "merge", "-q", "--ff-only", blocker)

	blockerPath := filepath.Join(store, "proj", blocker)
	waitUntil(t, "merged_at on "+blocker, func() bool { return !tickets.LoadEpicTicketMD(blockerPath).MergedAt.IsZero() })
	return func() tickets.Epic { return tickets.LoadEpicTicketMD(filepath.Join(store, "proj", mapName)) }
}

func TestMapDone_MergingTheImplEpicMarksItsMapDone(t *testing.T) {
	t.Parallel()
	load := handMergeEpic(t, "feat-impl", "feat")
	waitUntil(t, "map done", func() bool { m := load(); return m.Status == "done" && !m.MergedAt.IsZero() })
}

func TestMapDone_MergingAnotherEpicLeavesTheMapAlone(t *testing.T) {
	t.Parallel()
	load := handMergeEpic(t, "feat-other", "feat")
	time.Sleep(300 * time.Millisecond) // a negative: give a wrong stamp time to land
	if m := load(); m.Status != "open" || !m.MergedAt.IsZero() {
		t.Errorf("map status %q, merged_at %v, want untouched", m.Status, m.MergedAt)
	}
}
