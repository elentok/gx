package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// ticketFiles is every epic's ticket.md and ticket file in the store, by path.
func ticketFiles(t *testing.T, store string) map[string]string {
	t.Helper()
	var paths []string
	for _, pattern := range []string{"*/ticket.md", "*/issues/*.md"} {
		m, err := filepath.Glob(filepath.Join(store, "proj", pattern))
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, m...)
	}
	files := map[string]string{}
	for _, p := range paths {
		files[p] = readFile(t, p)
	}
	return files
}

func TestEpicMerge_RefusalsWriteNothing(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {Status: "done"}}, func(*schedFixture, servertest.Prompt, string) {
		t.Error("a refused merge launched an agent")
	})
	done := servertest.TicketOpts{Status: "done"}
	servertest.WriteTicketWith(t, f.store, "proj", "map-epic", "01", "t", done)
	testutil.WriteFile(t, filepath.Join(f.store, "proj", "map-epic"), "ticket.md", "---\nkind: map\nstatus: open\n---\n# Map\n")
	servertest.WriteTicketWith(t, f.store, "proj", "merged", "01", "t", done)
	testutil.WriteFile(t, filepath.Join(f.store, "proj", "merged"), "ticket.md", "---\nstatus: done\nmerged_at: 2026-01-02T03:04:05Z\n---\n# Merged\n")
	servertest.WriteTicketWith(t, f.store, "proj", "pending", "01", "t", done)
	servertest.WriteTicketWith(t, f.store, "proj", "pending", "02", "merge", servertest.TicketOpts{Type: "merge"})
	servertest.WriteTicketWith(t, f.store, "proj", "open", "01", "t", servertest.TicketOpts{})
	// The stored reason of a refused merge must survive it byte for byte.
	testutil.WriteFile(t, filepath.Join(f.store, "proj", "open"), "ticket.md", "---\nland_block_kind: dirty-trunk\nland_block_reason: trunk dirty\n---\n# Open\n")
	servertest.WriteTicketWith(t, f.store, "proj", server.OneOffsEpic, "01", "t", servertest.TicketOpts{})
	for _, branch := range []string{"map-epic", "merged", "pending", "open"} {
		gitOut(t, f.repo, "branch", branch)
	}
	f.h.Server.Rescan()
	before := ticketFiles(t, f.store)

	for addr, want := range map[string]string{
		"proj:" + schedEpic + "/01":  server.ReasonInvalidAddress,
		"::":                         server.ReasonInvalidAddress,
		"proj:nope":                  server.ReasonUnknownTicket,
		"nope:" + schedEpic:          server.ReasonUnknownTicket,
		"proj:map-epic":              server.ReasonMapEpic,
		"proj:" + server.OneOffsEpic: server.ReasonOneOffsEpic,
		"proj:merged":                server.ReasonAlreadyMerged,
		"proj:pending":               server.ReasonMergePending,
		"proj:open":                  server.ReasonNotComplete,
		"proj:" + schedEpic:          server.ReasonBranchMissing,
	} {
		res, err := f.h.Client.EpicMerge(context.Background(), addr)
		if err != nil || !res.Refused || res.Reason != want {
			t.Errorf("merge %s = %+v, %v, want refusal %s", addr, res, err, want)
		}
	}

	if q, err := f.h.Client.QueueItems(context.Background()); err != nil || len(q) != 0 {
		t.Errorf("queue = %+v, %v, want empty", q, err)
	}
	after := ticketFiles(t, f.store)
	for path, body := range after {
		if before[path] != body {
			t.Errorf("%s written by a refused merge:\n%s", path, body)
		}
	}
	if len(after) != len(before) {
		t.Errorf("ticket files = %d, want %d", len(after), len(before))
	}
}

func TestEpicMerge_CommitlessEpicWithNoBranchIsStampedMerged(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {Status: "done"}}, func(*schedFixture, servertest.Prompt, string) {})
	servertest.WriteTicketWith(t, f.store, "proj", "research", "01", "t", servertest.TicketOpts{Status: "done", Type: "research"})
	f.h.Server.Rescan()

	res, err := f.h.Client.EpicMerge(context.Background(), "proj:research")
	if err != nil || res.Refused || !res.Merged {
		t.Fatalf("merge = %+v, %v, want merged", res, err)
	}
	md := tickets.LoadEpicTicketMD(filepath.Join(f.store, "proj", "research"))
	if md.MergedAt.IsZero() {
		t.Error("epic has no merged_at")
	}
	if md.LinesAdded == nil || *md.LinesAdded != 0 || md.LinesRemoved == nil || *md.LinesRemoved != 0 {
		t.Errorf("lines = %v/%v, want 0/0", md.LinesAdded, md.LinesRemoved)
	}
}

// completedUnmerged runs the epic to its completion with auto-ff-merge off,
// returning the event stream.
func completedUnmerged(ctx context.Context, t *testing.T, f *schedFixture) <-chan server.Event {
	t.Helper()
	servertest.SetProjectRepoNoAutoMerge(t, f.store, "proj", f.repo)
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "proj:"+schedEpic)
	f.assertStamps(t, false)
	if f.rootQueued(t) {
		t.Fatal("root still queued after completing with auto-ff-merge off")
	}
	return evs
}

func newFFFixture(t *testing.T) *schedFixture {
	t.Helper()
	return newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
	})
}

func TestEpicMerge_FastForwardMergesDirectlyWithAutoFFMergeOff(t *testing.T) {
	f := newFFFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f)

	res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic)
	if err != nil || !res.Merged || res.Refused || res.MergeTicket != "" {
		t.Fatalf("merge = %+v, %v, want a direct merge", res, err)
	}
	if got := f.git(t, "rev-parse", "main"); got != f.git(t, "rev-parse", schedEpic) {
		t.Errorf("main at %s, want the feature branch tip", got)
	}
	f.assertStamps(t, true)
	if f.rootQueued(t) {
		t.Error("root queued after the direct merge")
	}
	if files := f.mergeTicketFiles(t); len(files) != 0 {
		t.Errorf("merge tickets = %v, want none", files)
	}
}

func TestEpicMerge_StampsLinesAddedAndRemovedFromTrunkBeforeToAfter(t *testing.T) {
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p servertest.Prompt, _ string) {
		// README.md is one line on trunk: this adds three and removes one.
		testutil.WriteFile(t, p.Cwd, "README.md", "a\nb\nc\n")
		testutil.CommitAll(t, p.Cwd, "rewrite readme")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f)

	if res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic); err != nil || !res.Merged {
		t.Fatalf("merge = %+v, %v, want a direct merge", res, err)
	}
	md := tickets.LoadEpicTicketMD(filepath.Join(f.store, "proj", schedEpic))
	if md.LinesAdded == nil || *md.LinesAdded != 3 || md.LinesRemoved == nil || *md.LinesRemoved != 1 {
		t.Errorf("lines = %v/%v, want 3/1", md.LinesAdded, md.LinesRemoved)
	}
}

func TestEpicMerge_RecreatesARemovedFeatureWorktree(t *testing.T) {
	f := newFFFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f)
	wt := f.featureWorktree(t)
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("no feature worktree to remove: %v", err)
	}
	f.git(t, "worktree", "remove", "--force", wt)

	res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic)
	if err != nil || !res.Merged {
		t.Fatalf("merge = %+v, %v, want a direct merge", res, err)
	}
	if got := gitOut(t, wt, "branch", "--show-current"); got != schedEpic {
		t.Errorf("recreated worktree on branch %q, want %q", got, schedEpic)
	}
}

func TestEpicMerge_DirtyTrunkPausesTheRootAndLeavesItQueued(t *testing.T) {
	f := newFFFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := completedUnmerged(ctx, t, f)
	// An untracked file in main's checkout that the merge would overwrite.
	testutil.WriteFile(t, f.repo, "01.txt", "local")

	res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic)
	if err != nil {
		t.Fatal(err)
	}
	assertMergePaused(t, res, events.DirtyTrunk)
	if !strings.Contains(res.Message, "01.txt") {
		t.Errorf("message = %q, want the blocking file", res.Message)
	}
	ev := servertest.WaitForEvent(ctx, t, evs, server.EventRootPaused, "proj:"+schedEpic)
	if ev.Kind != string(events.DirtyTrunk) {
		t.Errorf("kind = %q, want %q (%s)", ev.Kind, events.DirtyTrunk, ev.Message)
	}
	f.assertRootQueued(t)
	f.assertStamps(t, false)
}

func TestEpicMerge_MergeErrorReturnsAMergeErrorPause(t *testing.T) {
	f := newFFFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f)
	// A stale index lock fails the merge without any local change.
	testutil.WriteFile(t, filepath.Join(f.repo, ".git"), "index.lock", "")

	res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic)
	if err != nil {
		t.Fatal(err)
	}
	assertMergePaused(t, res, events.MergeError)
	if !strings.HasPrefix(res.Message, "merge failed: ") {
		t.Errorf("message = %q, want a merge failure", res.Message)
	}
	f.assertRootQueued(t)
	f.assertStamps(t, false)
}

// assertMergePaused checks the merge returned a pause of kind with a message.
func assertMergePaused(t *testing.T, res server.QueueResult, kind events.Kind) {
	t.Helper()
	if !res.Paused || res.Merged || res.Refused || res.MergeTicket != "" || res.PauseKind != string(kind) || res.Message == "" {
		t.Errorf("merge = %+v, want a %s pause with a message", res, kind)
	}
}

// raceTrunk makes the completed epic fast-forward, then moves main inside the
// merge, after the merge's ancestry check.
func raceTrunk(t *testing.T, f *forkFixture) {
	t.Helper()
	gitOut(t, f.featureWorktree(t), "rebase", "-q", "main")
	var merges atomic.Int32
	f.h.Server.SetMergeInto(func(repo git.Repo, wts []git.Worktree, branch, target string) (git.MergeOutcome, error) {
		if merges.Add(1) == 1 {
			moveMain(t, f.repo, "race")
		}
		return git.MergeBranchInto(repo, wts, branch, target)
	})
}

func TestEpicMerge_TrunkMovingDuringTheMergeForksANormalTicket(t *testing.T) {
	f := newForkFixture(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := completedUnmerged(ctx, t, f.schedFixture)
	raceTrunk(t, f)

	res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic)
	if err != nil || res.Refused || res.Merged || res.Paused || res.MergeTicket != "proj:"+schedEpic+"/02" {
		t.Fatalf("merge = %+v, %v, want merge ticket 02", res, err)
	}
	if body := readFile(t, f.ticketFile(t, "02")); strings.Contains(body, "manual:") {
		t.Errorf("race-forked ticket is manual:\n%s", body)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "proj:"+schedEpic)
}

func TestEpicMerge_TrunkMovingDuringTheMergeAtTheCapPauses(t *testing.T) {
	f := newForkFixtureWith(t, atMergeCap(map[string]servertest.TicketOpts{"01": {}}), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f.schedFixture)
	raceTrunk(t, f)

	res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic)
	if err != nil {
		t.Fatal(err)
	}
	assertMergePaused(t, res, "")
	if files := f.mergeTicketFiles(t); len(files) != 0 {
		t.Errorf("forked merge tickets = %v, want none past the cap", files)
	}
	f.assertRootQueued(t)
}

// assertManualTicket checks the merge returned the one manual merge ticket.
func (f *schedFixture) assertManualTicket(t *testing.T, res server.QueueResult, wantID string) {
	t.Helper()
	if res.Refused || res.Merged || res.MergeTicket != "proj:"+schedEpic+"/"+wantID {
		t.Fatalf("merge = %+v, want merge ticket %s", res, wantID)
	}
	if body := readFile(t, f.ticketFile(t, wantID)); !strings.Contains(body, "manual: true") || !strings.Contains(body, "type: merge") {
		t.Errorf("ticket %s is not a manual merge ticket:\n%s", wantID, body)
	}
}

func TestEpicMerge_NonFFForksAManualTicketWhoseFinishMergesTheEpic(t *testing.T) {
	f := newForkFixture(t, func(f *forkFixture, _ int) {
		if !f.rootQueued(t) {
			t.Error("root not re-queued while the manual ticket runs")
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := completedUnmerged(ctx, t, f.schedFixture)

	res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic)
	if err != nil {
		t.Fatal(err)
	}
	f.assertManualTicket(t, res, "02")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "proj:"+schedEpic)

	if got := f.merges.Load(); got != 1 {
		t.Errorf("merge turns = %d, want 1", got)
	}
	if got := f.git(t, "rev-parse", "main"); got != f.git(t, "rev-parse", schedEpic) {
		t.Errorf("main at %s, want the rebased feature branch tip", got)
	}
	f.assertStamps(t, true)
	if f.rootQueued(t) {
		t.Error("root still queued after the merge")
	}
}

func TestEpicMerge_ForkingAManualTicketClearsTheStoredBlock(t *testing.T) {
	f := newForkFixture(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completedUnmerged(ctx, t, f.schedFixture)
	block := map[string]any{"land_block_kind": "dirty-trunk", "land_block_reason": "trunk dirty"}
	if err := tickets.StampEpicFields(filepath.Join(f.store, "proj", schedEpic), block); err != nil {
		t.Fatal(err)
	}

	res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic)
	if err != nil {
		t.Fatal(err)
	}
	f.assertManualTicket(t, res, "02")
	if md := f.epicMD(t); strings.Contains(md, "land_block") {
		t.Errorf("fork left the block on the epic:\n%s", md)
	}
}

func TestEpicMerge_RetryTicketFinishingLastAfterTheManualOneStillMerges(t *testing.T) {
	f := newForkFixture(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := completedUnmerged(ctx, t, f.schedFixture)
	// Trunk moves once more before the manual ticket's merge, so a retry follows it.
	var merges atomic.Int32
	f.h.Server.SetMergeInto(func(repo git.Repo, wts []git.Worktree, branch, target string) (git.MergeOutcome, error) {
		if merges.Add(1) == 1 {
			moveMain(t, f.repo, "manual-merge")
		}
		return git.MergeBranchInto(repo, wts, branch, target)
	})

	res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic)
	if err != nil {
		t.Fatal(err)
	}
	f.assertManualTicket(t, res, "02")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "proj:"+schedEpic)

	if got := f.merges.Load(); got != 2 {
		t.Errorf("merge turns = %d, want 2", got)
	}
	if body := readFile(t, f.ticketFile(t, "03")); strings.Contains(body, "manual:") {
		t.Errorf("retry ticket is manual:\n%s", body)
	}
	if got := f.git(t, "rev-parse", "main"); got != f.git(t, "rev-parse", schedEpic) {
		t.Errorf("main at %s, want the rebased feature branch tip", got)
	}
	f.assertStamps(t, true)
}

func TestEpicMerge_ManualTicketResetsTheCapAndResumesTheRoot(t *testing.T) {
	f := newForkFixtureWith(t, atMergeCap(map[string]servertest.TicketOpts{"01": {}}), nil)
	ev := f.pauseRoot(t)
	if ev.Kind != "" {
		t.Fatalf("pause kind = %q (%s), want a needs-rebase pause", ev.Kind, ev.Message)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.subscribe(ctx, t)

	res, err := f.h.Client.EpicMerge(ctx, "proj:"+schedEpic)
	if err != nil {
		t.Fatal(err)
	}
	f.assertManualTicket(t, res, "04")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "proj:"+schedEpic)

	if got := f.merges.Load(); got != 1 {
		t.Errorf("merge turns = %d, want 1", got)
	}
	if got := f.git(t, "rev-parse", "main"); got != f.git(t, "rev-parse", schedEpic) {
		t.Errorf("main at %s, want the rebased feature branch tip", got)
	}
	f.assertStamps(t, true)
	if f.rootQueued(t) {
		t.Error("root still queued after the merge")
	}
	if got := len(f.mergeTicketFiles(t)); got != 1 {
		t.Errorf("forked merge tickets = %d, want only the manual one", got)
	}
}
