package diff_test

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/deps"
	iterwt "github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

// ticketRig is a server over a repo whose feature branch "epic" sits at the
// tip of main. Tickets are written after the start, so the startup reclaim
// leaves them alone.
type ticketRig struct {
	h     *servertest.Harness
	store string
	repo  string
	// root is the commit below the feature tip, which no iteration base resolves to.
	root string
	tip  string
}

func newTicketRig(t *testing.T) ticketRig {
	t.Helper()
	store := t.TempDir()
	repo := testutil.TempRepo(t)
	root := mustGit(t, repo, "rev-parse", "HEAD")
	commit(t, repo, "a.txt", "a\n", "add a")
	mustGit(t, repo, "branch", "epic")
	servertest.WriteTicketWith(t, store, project, "epic", "00", "seed", servertest.TicketOpts{})
	servertest.SetProjectRepoNoAutoMerge(t, store, project, repo)
	return ticketRig{
		h: servertest.StartWithStore(t, store), store: store, repo: repo,
		root: root, tip: mustGit(t, repo, "rev-parse", "HEAD"),
	}
}

func (r ticketRig) ticket(t *testing.T, id string, opts servertest.TicketOpts) {
	t.Helper()
	servertest.WriteTicketWith(t, r.store, project, "epic", id, "t"+id, opts)
}

// iteration adds ticket id's iteration worktree on from, with one commit.
func (r ticketRig) iteration(t *testing.T, id, from string) string {
	t.Helper()
	wtDir, err := deps.DefaultDeps().WorktreeDir(r.repo)
	if err != nil {
		t.Fatal(err)
	}
	path := iterwt.IterationWorktreePath(wtDir, "epic", id)
	mustGit(t, r.repo, "worktree", "add", "-q", "-b", iterwt.IterationBranch("epic", id), path, from)
	commit(t, path, "work"+id+".txt", id+"\n", "work "+id)
	return path
}

// ready waits until the server's ticket index has seen every ticket written so
// far: a last paused ticket becomes resolvable once the index reloads.
func (r ticketRig) ready(t *testing.T) {
	t.Helper()
	r.ticket(t, "99", servertest.TicketOpts{PauseReason: "iteration-error"})
	r.iteration(t, "99", "epic")
	servertest.WaitFor(t, 10*time.Second, "the server sees the tickets", func() bool {
		got, err := r.h.Client.Reads.DiffWorktree(context.Background(), project, "ticket:proj:epic/99")
		return err == nil && got.Reason == ""
	})
}

func (r ticketRig) track(id, base string) {
	r.h.Server.Registry().PutRunFrom(r.repo, runs.Run{Address: "proj:epic/" + id, Runner: "fake"}, base)
}

func filePaths(t *testing.T, h *servertest.Harness, got wire.DiffWorktree) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for p := range statuses(t, h, got) {
		out[p] = true
	}
	return out
}

func TestWorktreeTicketTrackedRunUsesItsOwnBase(t *testing.T) {
	t.Parallel()
	r := newTicketRig(t)
	wt := r.iteration(t, "01", "epic")
	r.track("01", r.root)
	r.ticket(t, "01", servertest.TicketOpts{Status: "claimed"})
	indexPath := filepath.Join(strings.TrimSpace(mustGit(t, wt, "rev-parse", "--absolute-git-dir")), "index")
	before, _ := os.ReadFile(indexPath)
	beforeStat, _ := os.Stat(indexPath)

	r.ready(t)

	got := worktree(t, r.h, "ticket:proj:epic/01")
	if got.Reason != "" || got.BaseSHA != r.root || got.BaseLabel != "epic@"+r.root[:7] {
		t.Fatalf("worktree = %+v, want base %s labelled epic@", got, r.root)
	}
	if got.TargetLabel != "proj:epic/01" {
		t.Errorf("target_label = %q, want the ticket address", got.TargetLabel)
	}
	if files := filePaths(t, r.h, got); !files["a.txt"] || !files["work01.txt"] {
		t.Errorf("files = %v, want the commit below the feature tip and the iteration's own", files)
	}
	after, _ := os.ReadFile(indexPath)
	afterStat, _ := os.Stat(indexPath)
	if sha256.Sum256(before) != sha256.Sum256(after) || !beforeStat.ModTime().Equal(afterStat.ModTime()) {
		t.Error("the iteration worktree's index changed")
	}
	if _, err := os.Stat(indexPath + ".lock"); !os.IsNotExist(err) {
		t.Errorf("index.lock left behind (stat err %v)", err)
	}
}

// stacked is a ticket 01 with a commit on its iteration branch and a ticket 02
// stacked on it, whose iteration worktree starts on 01's branch.
// A claimed ticket's run is tracked before its file is written, or the
// scheduler reopens the claim.
func (r ticketRig) stacked(t *testing.T, status string) (blockerTip string) {
	t.Helper()
	r.ticket(t, "01", servertest.TicketOpts{})
	r.iteration(t, "01", "epic")
	blockerTip = mustGit(t, r.repo, "rev-parse", iterwt.IterationBranch("epic", "01"))
	r.iteration(t, "02", iterwt.IterationBranch("epic", "01"))
	if status == "claimed" {
		r.track("02", blockerTip)
	}
	r.ticket(t, "02", servertest.TicketOpts{Status: status, BlockedBy: []string{"01"}})
	return blockerTip
}

func TestWorktreeTicketStackedLeavesOutTheBlockersCommits(t *testing.T) {
	t.Parallel()
	r := newTicketRig(t)
	blockerTip := r.stacked(t, "claimed")
	r.ready(t)

	got := worktree(t, r.h, "ticket:proj:epic/02")
	if got.Reason != "" || got.BaseSHA != blockerTip || got.BaseLabel != "proj:epic/01@"+blockerTip[:7] {
		t.Fatalf("worktree = %+v, want base %s labelled with the blocker's address", got, blockerTip)
	}
	if files := filePaths(t, r.h, got); !files["work02.txt"] || files["work01.txt"] {
		t.Errorf("files = %v, want only the iteration's own commit", files)
	}
}

func TestWorktreeTicketPausedWithoutRunDerivesItsBase(t *testing.T) {
	t.Parallel()
	r := newTicketRig(t)
	blockerTip := r.stacked(t, "paused")
	r.ticket(t, "03", servertest.TicketOpts{PauseReason: "iteration-error"})
	r.iteration(t, "03", "epic")
	r.ready(t)

	stacked := worktree(t, r.h, "ticket:proj:epic/02")
	if stacked.BaseSHA != blockerTip || stacked.BaseLabel != "proj:epic/01@"+blockerTip[:7] {
		t.Errorf("stacked = %+v, want the blocker branch's merge-base %s", stacked, blockerTip)
	}
	plain := worktree(t, r.h, "ticket:proj:epic/03")
	if plain.BaseSHA != r.tip || plain.BaseLabel != "epic@"+r.tip[:7] {
		t.Errorf("unstacked = %+v, want the feature merge-base %s", plain, r.tip)
	}

	// The blocker's branch is gone: fall back to the feature branch.
	wtDir, _ := deps.DefaultDeps().WorktreeDir(r.repo)
	mustGit(t, r.repo, "worktree", "remove", "--force", iterwt.IterationWorktreePath(wtDir, "epic", "01"))
	mustGit(t, r.repo, "branch", "-D", iterwt.IterationBranch("epic", "01"))
	fallback := worktree(t, r.h, "ticket:proj:epic/02")
	if fallback.BaseSHA != r.tip || fallback.BaseLabel != "epic@"+r.tip[:7] {
		t.Errorf("fallback = %+v, want the feature merge-base %s", fallback, r.tip)
	}
}

func TestWorktreeTicketEmptyStates(t *testing.T) {
	t.Parallel()
	r := newTicketRig(t)
	r.ticket(t, "01", servertest.TicketOpts{})
	r.iteration(t, "01", "epic")
	r.ticket(t, "02", servertest.TicketOpts{Status: "done"})
	r.iteration(t, "02", "epic")
	r.track("03", r.tip)
	r.ticket(t, "03", servertest.TicketOpts{Status: "claimed"})

	r.ready(t)

	for _, id := range []string{"01", "02", "03", "98"} {
		got := worktree(t, r.h, "ticket:proj:epic/"+id)
		if got.Reason != wire.DiffNoWorktree || got.SnapshotSHA != "" {
			t.Errorf("ticket %s = %+v, want reason %q", id, got, wire.DiffNoWorktree)
		}
	}
}

func TestWorktreeQueueAgentMatchesItsTicket(t *testing.T) {
	t.Parallel()
	h, row, release := rigs.StartQueueChat(t, "hello")
	defer release()

	want := worktree(t, h, "ticket:"+rigs.QueueRowTicket)
	got := worktree(t, h, "chat:"+row.ID)
	// Each read makes its own snapshot commit, so the shas are not comparable.
	hasSnap := got.SnapshotSHA != "" && want.SnapshotSHA != ""
	got.SnapshotSHA, want.SnapshotSHA = "", ""
	if want.Reason != "" || !hasSnap || got != want {
		t.Errorf("queue agent = %+v, want the ticket's %+v", got, want)
	}
}

func TestWorktreeQueueAgentWithoutRunHasNoDiff(t *testing.T) {
	t.Parallel()
	h, row, release := rigs.StartQueueChat(t, "hello")
	defer release()
	h.Server.Registry().Delete(rigs.QueueRowTicket)

	if got := worktree(t, h, "chat:"+row.ID); got.Reason != wire.DiffNoWorktree || got.SnapshotSHA != "" {
		t.Errorf("untracked queue agent = %+v, want reason %q", got, wire.DiffNoWorktree)
	}
}
