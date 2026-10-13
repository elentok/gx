package server_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/herdrfake"
)

// mergeTurn is what a merge agent saw on one turn.
type mergeTurn struct {
	prompt          servertest.Prompt
	branch          string
	iterationBranch string
}

// newMergeFixture runs a hand-placed merge ticket 01 in an epic whose feature
// branch exists. report is the iteration status the agent writes on turn n.
func newMergeFixture(t *testing.T, report func(n int) string) (f *schedFixture, turns func() []mergeTurn) {
	t.Helper()
	var mu sync.Mutex
	var seen []mergeTurn
	f = newSchedFixture(t, map[string]servertest.TicketOpts{"01": {Type: "merge"}}, func(f *schedFixture, p servertest.Prompt, _ string) {
		mu.Lock()
		seen = append(seen, mergeTurn{
			prompt:          p,
			branch:          gitOut(t, p.Cwd, "branch", "--show-current"),
			iterationBranch: strings.TrimSpace(f.git(t, "branch", "--list", "ralph-loop/*")),
		})
		n := len(seen)
		mu.Unlock()
		path := f.ticketFile(t, "01")
		body := strings.Replace(readFile(t, path), "status: claimed\n", "status: claimed\niteration_status: "+report(n)+"\n", 1)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Error(err)
		}
	})
	gitOut(t, f.repo, "branch", schedEpic)
	// Landing the epic is not this ticket's concern.
	servertest.SetProjectRepoNoAutoMerge(t, f.store, "proj", f.repo)
	return f, func() []mergeTurn {
		mu.Lock()
		defer mu.Unlock()
		return append([]mergeTurn(nil), seen...)
	}
}

func (f *schedFixture) featureWorktree(t *testing.T) string {
	t.Helper()
	wtDir, err := ralphloop.DefaultDeps().WorktreeDir(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wtDir, schedEpic)
}

// assertFeatureKept checks the feature worktree and the epic branch survived.
func (f *schedFixture) assertFeatureKept(t *testing.T, when string) {
	t.Helper()
	if _, err := os.Stat(f.featureWorktree(t)); err != nil {
		t.Errorf("feature worktree gone after %s: %v", when, err)
	}
	if strings.TrimSpace(f.git(t, "branch", "--list", schedEpic)) == "" {
		t.Errorf("branch %s gone after %s", schedEpic, when)
	}
}

func TestMergeTicket_RunsInTheFeatureWorktreeAndKeepsItWhenDone(t *testing.T) {
	f, turns := newMergeFixture(t, func(int) string { return "finished" })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	got := turns()
	if len(got) != 1 {
		t.Fatalf("turns = %d, want 1", len(got))
	}
	if want := "/gx-merge --ticket proj:" + schedEpic + "/01"; got[0].prompt.Text != want {
		t.Errorf("prompt = %q, want %q", got[0].prompt.Text, want)
	}
	if want := f.featureWorktree(t); got[0].prompt.Cwd != want {
		t.Errorf("cwd = %s, want the feature worktree %s", got[0].prompt.Cwd, want)
	}
	if got[0].branch != schedEpic {
		t.Errorf("agent ran on branch %q, want %q", got[0].branch, schedEpic)
	}
	if got[0].iterationBranch != "" {
		t.Errorf("iteration branch created for a merge ticket: %s", got[0].iterationBranch)
	}
	if body := readFile(t, f.ticketFile(t, "01")); !strings.Contains(body, "status: done") {
		t.Errorf("ticket not done:\n%s", body)
	}
	f.assertFeatureKept(t, "finish")
}

func TestMergeTicket_ParkKeepsTheFeatureWorktreeAndReclaimReusesIt(t *testing.T) {
	f, turns := newMergeFixture(t, func(n int) string {
		if n == 1 {
			return "needs-answer"
		}
		return "finished"
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventIterationParked, "proj:"+schedEpic+"/01")
	f.assertFeatureKept(t, "park")

	if err := ralphloop.UnparkTicket(f.ticketFile(t, "01"), time.Now()); err != nil {
		t.Fatal(err)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	got := turns()
	if len(got) != 2 {
		t.Fatalf("turns = %d, want 2", len(got))
	}
	if got[1].prompt.Cwd != got[0].prompt.Cwd {
		t.Errorf("reclaim ran in %s, want the same feature worktree %s", got[1].prompt.Cwd, got[0].prompt.Cwd)
	}
	f.assertFeatureKept(t, "reclaim")
}

func TestMergeTicket_FailedLaunchKeepsTheFeatureWorktree(t *testing.T) {
	f, _ := newMergeFixture(t, func(int) string { return "finished" })
	f.h.Herdr.Register("agent", "start", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return nil, herdrfake.Identities{}, errors.New("start refused")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventIterationLaunchFailed, "proj:"+schedEpic+"/01")

	f.assertFeatureKept(t, "discard")
}
