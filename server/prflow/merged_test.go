package prflow_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/ghfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// mergedRig is an epic with a stamped PR 5 that was squash-merged on origin.
func mergedRig(t *testing.T) (*rigs.PRFlow, string) {
	t.Helper()
	f := openRig(t, "")
	openPR(f)
	f.StampPR(t, 5, prURL)
	return f, f.Origin.SimulateSquashMerge("epic-a")
}

func count(ks []prflow.Kind, k prflow.Kind) int {
	n := 0
	for _, x := range ks {
		if x == k {
			n++
		}
	}
	return n
}

func TestMerged_SquashMergeFastForwardsTrunkThenStamps(t *testing.T) {
	f, sha := mergedRig(t)
	f.Tick(t)
	if e := tickets.LoadEpicAt(f.EpicDir()); e.MergedAt.IsZero() || e.MergeBlockKind != "" {
		t.Fatalf("merged_at %v, block %q", e.MergedAt, e.MergeBlockKind)
	}
	if got := strings.TrimSpace(servertest.GitOut(t, f.Origin.Local, "rev-parse", "main")); got != sha {
		t.Errorf("local main = %s, want the squash commit %s", got, sha)
	}
	if len(f.Origin.Fetches()) == 0 {
		t.Error("no fetch reached origin")
	}
	want := []prflow.Kind{prflow.KindPRMerged, prflow.KindTrunkUpdated}
	if got := kinds(t, f); !slices.Equal(got, want) {
		t.Errorf("activity = %v, want %v", got, want)
	}
}

// twoCommitRig is an epic with a stamped PR 5 whose branch has two commits, not
// yet merged. It returns trunk's tip, the PR's fork point.
func twoCommitRig(t *testing.T) (*rigs.PRFlow, string) {
	t.Helper()
	f := openRig(t, "")
	repo := f.Origin.Local
	fork := gitRev(t, repo, "main")
	servertest.RunGitIn(t, repo, "checkout", "-q", "epic-a")
	testutil.WriteFile(t, repo, "second.txt", "2")
	testutil.CommitAll(t, repo, "second")
	servertest.RunGitIn(t, repo, "checkout", "-q", "main")
	openPR(f)
	f.StampPR(t, 5, prURL)
	return f, fork
}

func gitRev(t *testing.T, repo, rev string) string {
	t.Helper()
	return strings.TrimSpace(servertest.GitOut(t, repo, "rev-parse", rev))
}

// wantRange asserts the epic is merged and stamped with base and tip; empty
// both means merged with no range.
func wantRange(t *testing.T, f *rigs.PRFlow, base, tip string) {
	t.Helper()
	e := tickets.LoadEpicAt(f.EpicDir())
	if e.MergedAt.IsZero() {
		t.Fatal("merged_at not stamped")
	}
	if e.RangeBase != base || e.RangeTip != tip {
		t.Fatalf("range = %q..%q, want %q..%q", e.RangeBase, e.RangeTip, base, tip)
	}
}

func TestMerged_MergeCommitStampsItsFirstParentAndItself(t *testing.T) {
	f, fork := twoCommitRig(t)
	m := f.Origin.SimulateMergeCommit("epic-a")
	f.Tick(t)
	wantRange(t, f, fork, m)
}

func TestMerged_SquashStampsItsParentAndItself(t *testing.T) {
	f, fork := twoCommitRig(t)
	m := f.Origin.SimulateSquashMerge("epic-a")
	f.Tick(t)
	wantRange(t, f, fork, m)
}

func TestMerged_SquashWithoutTheMarkStampsNoRange(t *testing.T) {
	f, _ := twoCommitRig(t)
	f.Origin.SimulateUnmarkedSquashMerge("epic-a")
	f.Tick(t)
	wantRange(t, f, "", "")
}

func TestMerged_RebaseStampsTheCommitBeforeTheRunAndItsLast(t *testing.T) {
	f, fork := twoCommitRig(t)
	last := f.Origin.SimulateRebaseMerge("epic-a", false)
	f.Tick(t)
	wantRange(t, f, fork, last)
}

func TestMerged_RebaseWithRewordedHeadlinesStampsNoRange(t *testing.T) {
	f, _ := twoCommitRig(t)
	f.Origin.SimulateRebaseMerge("epic-a", true)
	f.Tick(t)
	wantRange(t, f, "", "")
}

func TestMerged_MergeCommitSurvivesABlockedTrunkUpdateAndItsRetry(t *testing.T) {
	f, fork := twoCommitRig(t)
	clash := filepath.Join(f.Origin.Local, "second.txt")
	if err := os.WriteFile(clash, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := f.Origin.SimulateMergeCommit("epic-a")
	f.Tick(t)
	if !tickets.LoadEpicAt(f.EpicDir()).MergedAt.IsZero() {
		t.Fatal("stamped despite the dirty trunk")
	}
	// GitHub no longer reports the PR: the retry has only the activity log.
	f.GH.Update(func(s *ghfake.State) { s.PRs[0].MergeCommit = "" })
	if err := os.Remove(clash); err != nil {
		t.Fatal(err)
	}
	f.Tick(t)
	wantRange(t, f, fork, m)
}

func TestMerged_DirtyTrunkBlocksRetriesAndWritesOneEntry(t *testing.T) {
	f, _ := mergedRig(t)
	clash := filepath.Join(f.Origin.Local, "epic-a.txt")
	if err := os.WriteFile(clash, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.Tick(t)
	f.Tick(t)
	e := tickets.LoadEpicAt(f.EpicDir())
	if !e.MergedAt.IsZero() || e.MergeBlockKind != string(schema.PausePRDirtyTrunk) || !slices.Equal(e.MergeBlockFiles, []string{"epic-a.txt"}) {
		t.Fatalf("merged_at %v, block %q files %v", e.MergedAt, e.MergeBlockKind, e.MergeBlockFiles)
	}
	if ks := kinds(t, f); count(ks, prflow.KindPRMerged) != 1 || count(ks, prflow.KindTrunkUpdated) != 0 {
		t.Fatalf("activity = %v", ks)
	}
	if err := os.Remove(clash); err != nil {
		t.Fatal(err)
	}
	f.Tick(t)
	if e := tickets.LoadEpicAt(f.EpicDir()); e.MergedAt.IsZero() || e.MergeBlockKind != "" {
		t.Fatalf("after cleaning, merged_at %v, block %q", e.MergedAt, e.MergeBlockKind)
	}
	if ks := kinds(t, f); count(ks, prflow.KindPRMerged) != 1 || count(ks, prflow.KindTrunkUpdated) != 1 {
		t.Fatalf("activity = %v", ks)
	}
}

func TestMerged_DivergedTrunkStoresTrunkUpdateBlock(t *testing.T) {
	f, _ := mergedRig(t)
	servertest.RunGitIn(t, f.Origin.Local, "commit", "-q", "--allow-empty", "-m", "local only")
	f.Tick(t)
	e := tickets.LoadEpicAt(f.EpicDir())
	if !e.MergedAt.IsZero() || e.MergeBlockKind != string(schema.PausePRTrunkUpdate) || e.MergeBlockReason == "" {
		t.Fatalf("merged_at %v, block %q reason %q", e.MergedAt, e.MergeBlockKind, e.MergeBlockReason)
	}
}

func TestMerged_RestartAfterTheEntryRetriesWithoutASecondEntry(t *testing.T) {
	f, sha := mergedRig(t)
	if err := prflow.Append(f.EpicDir(), prflow.Entry{Kind: prflow.KindPRMerged}); err != nil {
		t.Fatal(err)
	}
	f.Tick(t)
	if got := strings.TrimSpace(servertest.GitOut(t, f.Origin.Local, "rev-parse", "main")); got != sha {
		t.Errorf("local main = %s, want %s", got, sha)
	}
	want := []prflow.Kind{prflow.KindPRMerged, prflow.KindTrunkUpdated}
	if got := kinds(t, f); !slices.Equal(got, want) {
		t.Errorf("activity = %v, want %v", got, want)
	}
}

func TestMerged_CancelsUnfinishedCIFixAndClearsCIFailing(t *testing.T) {
	f, _ := mergedRig(t)
	servertest.WriteTicketWith(t, f.Store, "proj", "epic-a", "02", "fix", servertest.TicketOpts{Type: "ci-fix"})
	servertest.WriteTicketWith(t, f.Store, "proj", "epic-a", "03", "fix", servertest.TicketOpts{Type: "ci-fix", PauseReason: "manual"})
	if err := tickets.StampEpicFields(f.EpicDir(), map[string]any{"land_block_kind": "ci-failing", "land_block_reason": "red"}); err != nil {
		t.Fatal(err)
	}
	f.Rescan(t)
	f.Tick(t)
	for _, tk := range tickets.LoadEpicAt(f.EpicDir()).Tickets {
		if tk.Type == string(schema.TypeCIFix) && tk.Status != string(schema.StatusCancelled) {
			t.Errorf("ci-fix %s is %s, want cancelled", tk.Identifier, tk.Status)
		}
	}
	if e := tickets.LoadEpicAt(f.EpicDir()); e.MergeBlockKind != "" || e.MergedAt.IsZero() {
		t.Errorf("block %q, merged_at %v", e.MergeBlockKind, e.MergedAt)
	}
}

func TestClosed_ClearsTheMergeBlockToAwaitingPR(t *testing.T) {
	f := openRig(t, "")
	openPR(f)
	f.StampPR(t, 5, prURL)
	if err := tickets.StampEpicFields(f.EpicDir(), map[string]any{"land_block_kind": "ci-failing", "land_block_reason": "red"}); err != nil {
		t.Fatal(err)
	}
	f.GH.Update(func(s *ghfake.State) { s.PRs[0].State = "CLOSED" })
	f.Rescan(t)
	f.Tick(t)
	e := tickets.LoadEpicAt(f.EpicDir())
	if e.PRNumber != 0 || e.MergeBlockKind != "" || e.MergeBlockReason != "" {
		t.Fatalf("pr %d, block %q reason %q", e.PRNumber, e.MergeBlockKind, e.MergeBlockReason)
	}
}

func TestClosed_ClearsThePRAndLogsIt(t *testing.T) {
	f := openRig(t, "")
	openPR(f)
	f.StampPR(t, 5, prURL)
	f.GH.Update(func(s *ghfake.State) { s.PRs[0].State = "CLOSED" })
	f.Tick(t)
	e := tickets.LoadEpicAt(f.EpicDir())
	if e.PRNumber != 0 || e.PRURL != "" || !e.MergedAt.IsZero() {
		t.Fatalf("pr %d %q merged_at %v", e.PRNumber, e.PRURL, e.MergedAt)
	}
	if got := kinds(t, f); !slices.Equal(got, []prflow.Kind{prflow.KindPRClosed}) {
		t.Errorf("activity = %v", got)
	}
}
