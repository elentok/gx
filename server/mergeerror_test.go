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

// pauseRoot runs the epic to its root pause, failing on an iteration-failed on
// the way, and returns the pause event.
func (f *schedFixture) pauseRoot(t *testing.T) server.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	for {
		select {
		case ev, open := <-evs:
			if !open {
				t.Fatal("event stream ended before root-paused")
			}
			switch ev.Type {
			case server.EventIterationFailed:
				t.Fatalf("iteration-failed on a merge error: %+v", ev)
			case server.EventRootPaused:
				return ev
			}
		case <-ctx.Done():
			t.Fatalf("no root-paused: %v", ctx.Err())
		}
	}
}

// assertRootQueued checks the paused root was not dequeued.
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

func TestMergeError_DirtyTrunkPausesTheRootWithACappedFileList(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	files := []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt", "f.txt", "g.txt"}
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, _ string) {
		commitFiles(t, p, files...)
		// Untracked files in main's checkout that the merge would overwrite.
		for _, n := range files {
			testutil.WriteFile(t, f.repo, n, "local "+n)
		}
	}, chat)
	main := f.git(t, "rev-parse", "main")

	ev := f.pauseRoot(t)

	if ev.Kind != string(events.DirtyTrunk) {
		t.Errorf("kind = %q, want %q", ev.Kind, events.DirtyTrunk)
	}
	want := fmt.Sprintf("uncommitted changes on main (%s) clash with a.txt, b.txt, c.txt, d.txt, e.txt +2 more; "+
		"commit or discard them, then merge again", f.repo)
	if ev.Message != want {
		t.Errorf("reason =\n%q\nwant\n%q", ev.Message, want)
	}
	if got := f.git(t, "rev-parse", "main"); got != main {
		t.Errorf("main moved to %s", got)
	}
	f.assertRootQueued(t)
	f.assertStamps(t, false)
	if got := waitForChat(count, "clash with a.txt"); got != 1 {
		t.Errorf("pause sent %d times, want once", got)
	}
	if got := count(epicNotMergedText) + count(epicMergedText); got != 0 {
		t.Errorf("epic notice sent %d times, want none", got)
	}
}

func TestMergeError_DirtyTrunkNamesOnlyTheClashingFiles(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, _ string) {
		commitFiles(t, p, "clash.txt")
		testutil.WriteFile(t, f.repo, "clash.txt", "local")
		testutil.WriteFile(t, f.repo, "unrelated.txt", "local")
	})

	ev := f.pauseRoot(t)

	want := fmt.Sprintf("uncommitted changes on main (%s) clash with clash.txt; commit or discard them, then merge again", f.repo)
	if ev.Kind != string(events.DirtyTrunk) || ev.Message != want {
		t.Errorf("pause = kind %q, reason %q, want reason %q", ev.Kind, ev.Message, want)
	}
}

func TestMergeError_DirtyTrunkListsNewDirectoryFilesAndBothRenamePaths(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, _ string) {
		testutil.WriteFile(t, p.Cwd, "old.txt", "old")
		testutil.CommitAll(t, p.Cwd, "add old")
		for _, root := range []string{p.Cwd, f.repo} {
			if err := os.Mkdir(filepath.Join(root, "newdir"), 0o755); err != nil && !os.IsExist(err) {
				t.Fatal(err)
			}
		}
		testutil.WriteFile(t, p.Cwd, "newdir/x.txt", "x")
		testutil.MustGitExported(t, p.Cwd, "mv", "old.txt", "new.txt")
		testutil.CommitAll(t, p.Cwd, "work")

		// Main's checkout holds the same paths locally.
		testutil.WriteFile(t, f.repo, "newdir/x.txt", "local")
		testutil.WriteFile(t, f.repo, "new.txt", "local")
	})

	ev := f.pauseRoot(t)

	want := fmt.Sprintf("uncommitted changes on main (%s) clash with new.txt, newdir/x.txt; commit or discard them, then merge again", f.repo)
	if ev.Message != want {
		t.Errorf("reason =\n%q\nwant\n%q", ev.Message, want)
	}
}

// dirtyTrunkFixture pauses the epic on a dirty trunk; clean undoes the dirt.
func dirtyTrunkFixture(t *testing.T) (f *schedFixture, clean func()) {
	t.Helper()
	turns := 0
	f = newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, _ string) {
		turns++
		commitFiles(t, p, fmt.Sprintf("a%d.txt", turns), "a.txt")
		testutil.WriteFile(t, f.repo, "a.txt", "local")
	})
	return f, func() {
		if err := os.Remove(filepath.Join(f.repo, "a.txt")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMergeError_PauseStoresKindAndReasonUntilALaterMergeClearsThem(t *testing.T) {
	t.Parallel()
	f, clean := dirtyTrunkFixture(t)
	ev := f.pauseRoot(t)

	md := f.epicMD(t)
	if !strings.Contains(md, "land_block_kind: "+string(events.DirtyTrunk)) || !strings.Contains(md, "land_block_reason:") {
		t.Errorf("pause not stored on the epic:\n%s", md)
	}
	if !strings.Contains(md, "clash with") || ev.Message == "" {
		t.Errorf("reason not stored:\n%s", md)
	}

	clean()
	res, err := f.h.Client.EpicMerge(context.Background(), "proj:"+schedEpic)
	if err != nil || !res.Merged {
		t.Fatalf("merge = %+v, %v", res, err)
	}
	if md := f.epicMD(t); strings.Contains(md, "land_block") {
		t.Errorf("merge left the block on the epic:\n%s", md)
	}
}

func TestMergeError_DirtyTrunkStoresTheFullFileListAndForksNoInvestigation(t *testing.T) {
	t.Parallel()
	files := []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt", "f.txt", "g.txt"}
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, _ string) {
		commitFiles(t, p, files...)
		for _, n := range files {
			testutil.WriteFile(t, f.repo, n, "local "+n)
		}
	})
	f.pauseRoot(t)

	md := f.epicMD(t)
	for _, n := range files {
		if !strings.Contains(md, "- "+n+"\n") {
			t.Errorf("land_block_files lacks %s:\n%s", n, md)
		}
	}
	for _, n := range files {
		if got := readFile(t, filepath.Join(f.repo, n)); got != "local "+n {
			t.Errorf("%s = %q, want the local content untouched", n, got)
		}
	}
	entries, err := os.ReadDir(f.issues)
	if err != nil || len(entries) != 1 {
		t.Errorf("epic issues = %v, %v; want only ticket 01 (no investigation)", entries, err)
	}
}

func TestMergeError_MergeAgainPausesWithAFreshListThenLandsOnceClean(t *testing.T) {
	t.Parallel()
	f, clean := dirtyTrunkFixture(t)
	f.pauseRoot(t)

	res, err := f.h.Client.EpicMerge(context.Background(), "proj:"+schedEpic)
	if err != nil || !res.Paused || res.PauseKind != string(events.DirtyTrunk) {
		t.Fatalf("merge again = %+v, %v; want a dirty-trunk pause", res, err)
	}
	if md := f.epicMD(t); strings.Count(md, "land_block_kind") != 1 || !strings.Contains(md, "- a.txt\n") {
		t.Errorf("pause not rewritten once:\n%s", md)
	}

	clean()
	res, err = f.h.Client.EpicMerge(context.Background(), "proj:"+schedEpic)
	if err != nil || !res.Merged {
		t.Fatalf("merge after cleaning = %+v, %v", res, err)
	}
	if md := f.epicMD(t); strings.Contains(md, "land_block") {
		t.Errorf("landing left the block on the epic:\n%s", md)
	}
}

func TestMergeError_DefaultPathFinishClearsTheStoredBlock(t *testing.T) {
	t.Parallel()
	f, _ := dirtyTrunkFixture(t)
	f.pauseRoot(t)

	servertest.SetProjectRepoNoAutoMerge(t, f.store, "proj", f.repo)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := f.h.Client.QueueRemove(ctx, "proj:"+schedEpic+"/01"); err != nil {
		t.Fatal(err)
	}
	// Reopen the ticket, so its second finish runs the merge step again.
	path := f.ticketFile(t, "01")
	reopened := strings.Replace(readFile(t, path), "status: done", "status: open", 1)
	testutil.WriteFile(t, filepath.Dir(path), filepath.Base(path), reopened)
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	if md := f.epicMD(t); strings.Contains(md, "land_block") {
		t.Errorf("default-path finish left the block on the epic:\n%s", md)
	}
}

func TestMergeError_NonOverlappingLocalChangeDoesNotBlockTheMerge(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
		testutil.WriteFile(t, f.repo, "unrelated.txt", "local")
	})
	f.runEpic(t, server.EventRootCompleted)

	if got := f.git(t, "rev-parse", "main"); got != f.git(t, "rev-parse", schedEpic) {
		t.Errorf("main at %s, want the feature branch tip", got)
	}
}

func TestMergeError_OtherGitFailurePausesMergeFailed(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
		// A stale index lock fails the merge without any local change.
		if err := os.WriteFile(filepath.Join(f.repo, ".git", "index.lock"), nil, 0o644); err != nil {
			t.Error(err)
		}
	})

	ev := f.pauseRoot(t)

	if ev.Kind != string(events.MergeError) {
		t.Errorf("kind = %q, want %q", ev.Kind, events.MergeError)
	}
	if !strings.HasPrefix(ev.Message, "merge failed: ") || !strings.Contains(ev.Message, "index.lock") ||
		!strings.HasSuffix(ev.Message, "; fix it, then merge again") {
		t.Errorf("reason = %q", ev.Message)
	}
	f.assertRootQueued(t)
	f.assertStamps(t, false)
}
