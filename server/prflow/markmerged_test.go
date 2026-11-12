package prflow_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/prflow"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/ghfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func markMerged(t *testing.T, f *rigs.PRFlow) wire.QueueResult {
	t.Helper()
	res, err := f.H.Client.Tickets.EpicMarkMerged(t.Context(), "proj:epic-a")
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// handMergedRig is a PR that was merged outside gx while GitHub still reports
// it OPEN, so the watcher's own merged path cannot race the verb under test.
func handMergedRig(t *testing.T) (*rigs.PRFlow, string) {
	t.Helper()
	f, sha := mergedRig(t)
	f.GH.Update(func(s *ghfake.State) { s.PRs[0].State = "OPEN" })
	return f, sha
}

func TestMarkMerged_StampsAMergeProjectEpicAndASecondCallIsANoOp(t *testing.T) {
	f := openRig(t, "")
	setProject(t, f, deliveryMerge)

	if res := markMerged(t, f); res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	first := tickets.LoadEpicAt(f.EpicDir()).MergedAt
	if first.IsZero() {
		t.Fatal("merged_at not stamped")
	}
	if res := markMerged(t, f); res.Refused {
		t.Fatalf("second call refused: %+v", res)
	}
	if got := tickets.LoadEpicAt(f.EpicDir()).MergedAt; !got.Equal(first) {
		t.Errorf("second call moved merged_at from %v to %v", first, got)
	}
}

func TestMarkMerged_RefusesUnfinishedWork(t *testing.T) {
	cases := map[string]struct {
		ticket servertest.TicketOpts
		queue  bool
		want   string
	}{
		"incomplete": {servertest.TicketOpts{}, false, wire.ReasonNotComplete},
		"running":    {servertest.TicketOpts{Status: "claimed"}, false, wire.ReasonTicketLive},
		"queued":     {servertest.TicketOpts{}, true, wire.ReasonTicketLive},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := openRig(t, "")
			setProject(t, f, deliveryMerge)
			servertest.WriteTicketWith(t, f.Store, "proj", "epic-a", "02", "more", c.ticket)
			f.Rescan(t)
			if c.queue {
				if res, err := f.H.Client.Queue.Add(t.Context(), "proj:epic-a/02", "claude"); err != nil || res.Refused {
					t.Fatalf("queue add: %+v %v", res, err)
				}
			}
			res := markMerged(t, f)
			if !res.Refused || res.Reason != c.want {
				t.Fatalf("result = %+v, want refusal %q", res, c.want)
			}
			if !tickets.LoadEpicAt(f.EpicDir()).MergedAt.IsZero() {
				t.Error("merged_at stamped on a refused epic")
			}
		})
	}
}

func TestMarkMerged_OpenPRStopsWatchingCancelsFixesAndStampsAfterTheFastForward(t *testing.T) {
	f, sha := handMergedRig(t)
	servertest.WriteTicketWith(t, f.Store, "proj", "epic-a", "02", "fix", servertest.TicketOpts{Type: "ci-fix"})
	f.Rescan(t)

	if res := markMerged(t, f); res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	if e := tickets.LoadEpicAt(f.EpicDir()); e.MergedAt.IsZero() || e.MergeBlockKind != "" {
		t.Fatalf("merged_at %v, block %q", e.MergedAt, e.MergeBlockKind)
	}
	if got := strings.TrimSpace(servertest.GitOut(t, f.Origin.Local, "rev-parse", "main")); got != sha {
		t.Errorf("local main = %s, want %s", got, sha)
	}
	for _, tk := range tickets.LoadEpicAt(f.EpicDir()).Tickets {
		if tk.Type == string(schema.TypeCIFix) && tk.Status != string(schema.StatusCancelled) {
			t.Errorf("ci-fix %s is %s, want cancelled", tk.Identifier, tk.Status)
		}
	}
	want := []prflow.Kind{prflow.KindMarkedMerged, prflow.KindTrunkUpdated}
	if got := kinds(t, f); !slices.Equal(got, want) {
		t.Errorf("activity = %v, want %v (no \"PR merged\")", got, want)
	}
	f.Tick(t)
	if got := kinds(t, f); !slices.Equal(got, want) {
		t.Errorf("after a tick, activity = %v, want %v", got, want)
	}
}

func TestStashMerge_RefusesWithLocalAheadWhenLocalMainHasCommitsOriginLacks(t *testing.T) {
	f, _ := handMergedRig(t)
	if err := os.WriteFile(filepath.Join(f.Origin.Local, "epic-a.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if res := markMerged(t, f); res.Reason != string(schema.PausePRDirtyTrunk) {
		t.Fatalf("mark-merged = %+v, want a pr-dirty-trunk pause", res)
	}
	if err := os.WriteFile(filepath.Join(f.Origin.Local, "ahead.txt"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.GitOut(t, f.Origin.Local, "add", "ahead.txt")
	servertest.GitOut(t, f.Origin.Local, "commit", "-q", "-m", "local only")

	res, err := f.H.Client.Tickets.EpicStashMerge(t.Context(), "proj:epic-a")

	if err != nil || res.Reason != wire.ReasonLocalAhead {
		t.Errorf("stash-merge = %+v, %v, want the local-ahead refusal", res, err)
	}
}

func TestMarkMerged_DirtyTrunkKeepsMergedAtEmptyUntilTheTrunkIsClean(t *testing.T) {
	f, _ := handMergedRig(t)
	clash := filepath.Join(f.Origin.Local, "epic-a.txt")
	if err := os.WriteFile(clash, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := markMerged(t, f)
	if !res.Refused || res.Reason != string(schema.PausePRDirtyTrunk) {
		t.Fatalf("result = %+v, want a pr-dirty-trunk refusal", res)
	}
	if e := tickets.LoadEpicAt(f.EpicDir()); !e.MergedAt.IsZero() || e.MergeBlockKind != string(schema.PausePRDirtyTrunk) {
		t.Fatalf("merged_at %v, block %q", e.MergedAt, e.MergeBlockKind)
	}

	if err := os.Remove(clash); err != nil {
		t.Fatal(err)
	}
	f.Tick(t)
	if e := tickets.LoadEpicAt(f.EpicDir()); e.MergedAt.IsZero() || e.MergeBlockKind != "" {
		t.Errorf("after cleaning, merged_at %v, block %q", e.MergedAt, e.MergeBlockKind)
	}
}

// handMergedChatRig is handMergedRig with chat notices counted.
func handMergedChatRig(t *testing.T) (*rigs.PRFlow, func(string) int) {
	t.Helper()
	f, count := chatRig(t)
	servertest.WriteTicketWith(t, f.Store, "proj", "epic-a", "01", "root", servertest.TicketOpts{Status: "done"})
	servertest.CommitOnBranch(t, f.Origin.Local, "epic-a")
	openPR(f)
	f.StampPR(t, 5, prURL)
	f.Origin.SimulateSquashMerge("epic-a")
	return f, count
}

func TestMarkMerged_SendsNoMergedNoticeButAWatcherDetectedMergeDoes(t *testing.T) {
	hand, handCount := handMergedChatRig(t)
	hand.GH.Update(func(s *ghfake.State) { s.PRs[0].State = "OPEN" })
	if res := markMerged(t, hand); res.Refused {
		t.Fatalf("refused: %+v", res)
	}
	if tickets.LoadEpicAt(hand.EpicDir()).MergedAt.IsZero() {
		t.Fatal("merged_at not stamped")
	}

	// The watcher-detected merge is the control: once its notice has flushed,
	// the hand merge's would have too.
	watched, watchedCount := handMergedChatRig(t)
	watched.Tick(t)
	if got := rigs.WaitForChat(watchedCount, rigs.EpicMergedText); got != 1 {
		t.Fatalf("watcher-detected merge sent %d merged notices, want 1", got)
	}
	time.Sleep(2 * servertest.FlushInterval) // negative wait
	if got := handCount(rigs.EpicMergedText); got != 0 {
		t.Errorf("hand Mark as merged sent %d merged notices, want none", got)
	}
}
