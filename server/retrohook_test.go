package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// completeEpic runs the fixture's epic to completion and returns the retro
// one-off epics the hook submitted in the project by then.
func (f *schedFixture) completeEpic(t *testing.T) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")
	retros, err := filepath.Glob(filepath.Join(f.Store, "proj", queue.OneOffsEpic, "issues", "*-evolve-"+schedEpic+"-*.md"))
	if err != nil {
		t.Fatal(err)
	}
	return retros
}

func retroFixture(t *testing.T, opts ...func(*server.Config)) *schedFixture {
	t.Helper()
	return newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p *servertest.Turn, id string) {
		// A submitted retro launches too; it is commitless, so it commits nothing.
		if strings.Contains(p.Address, ":one-offs/") {
			return
		}
		commitWork(t, p, id)
	}, opts...)
}

func TestRetroHook_SubmitsAfterTheEpicMerges(t *testing.T) {
	t.Parallel()
	f := retroFixture(t, servertest.WithEvolve(config.EvolveContributor))
	if got := f.completeEpic(t); len(got) != 1 {
		t.Errorf("retros = %v, want one", got)
	}
}

func TestRetroHook_NothingWhenEvolveIsOff(t *testing.T) {
	t.Parallel()
	f := retroFixture(t, servertest.WithEvolve(config.EvolveOff))
	if got := f.completeEpic(t); len(got) != 0 {
		t.Errorf("retros = %v, want none", got)
	}
}

func TestRetroHook_NothingWhenTheProjectOptedOut(t *testing.T) {
	t.Parallel()
	f := retroFixture(t, servertest.WithEvolve(config.EvolveContributor))
	dir := filepath.Join(f.Store, "proj")
	pf, err := config.ReadProjectFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	no := false
	pf.Evolve = &no
	if err := tickets.WriteProjectFile(dir, pf); err != nil {
		t.Fatal(err)
	}
	if got := f.completeEpic(t); len(got) != 0 {
		t.Errorf("retros = %v, want none", got)
	}
}

func TestRetroHook_NothingWhenTheEpicWasAlreadyEvolved(t *testing.T) {
	t.Parallel()
	f := retroFixture(t, servertest.WithEvolve(config.EvolveContributor))
	if err := tickets.StampEpicFields(filepath.Join(f.Store, "proj", schedEpic), map[string]any{"evolved_at": time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if got := f.completeEpic(t); len(got) != 0 {
		t.Errorf("retros = %v, want none", got)
	}
}

func TestRetroHook_MaintainerWithoutAGxProjectSubmitsNothing(t *testing.T) {
	t.Parallel()
	f := retroFixture(t, servertest.WithEvolve(config.EvolveMaintainer))
	if got := f.completeEpic(t); len(got) != 0 {
		t.Errorf("retros = %v, want none", got)
	}
}

func TestRetroHook_ContributorCreatesTheOutboxProject(t *testing.T) {
	t.Parallel()
	f := retroFixture(t, servertest.WithEvolve(config.EvolveContributor))
	if got := f.completeEpic(t); len(got) != 1 {
		t.Fatalf("retros = %v, want one", got)
	}
	pf, err := config.ReadProjectFile(filepath.Join(f.Store, "gx"))
	if err != nil {
		t.Fatalf("outbox project not created: %v", err)
	}
	if pf.VCS == nil || *pf.VCS != config.VCSNone {
		t.Errorf("outbox vcs = %v, want none", pf.VCS)
	}
}

func TestRetroHook_MaintainerWithAGxProjectSubmits(t *testing.T) {
	t.Parallel()
	f := retroFixture(t, servertest.WithEvolve(config.EvolveMaintainer))
	if err := os.MkdirAll(filepath.Join(f.Store, "gx"), 0o755); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, f.Store, "gx", f.Repo)
	if got := f.completeEpic(t); len(got) != 1 {
		t.Errorf("retros = %v, want one", got)
	}
}

// retrosAfter waits for the one retro the hook submits after a root event: the
// submit follows the pause/fail event, so the glob is polled.
func (f *schedFixture) retrosAfter(t *testing.T) []string {
	t.Helper()
	var retros []string
	servertest.Poll(10*time.Second, func() bool {
		var err error
		retros, err = filepath.Glob(filepath.Join(f.Store, "proj", queue.OneOffsEpic, "issues", "*-evolve-"+schedEpic+"-*.md"))
		if err != nil {
			t.Fatal(err)
		}
		return len(retros) > 0
	})
	return retros
}

func TestRetroHook_SubmitsWhenTheMergeNeedsARebase(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, atMergeCap(map[string]servertest.TicketOpts{"01": {}}), func(f *schedFixture, p *servertest.Turn, id string) {
		if strings.Contains(p.Address, ":one-offs/") {
			return
		}
		commitWork(t, p, id)
		testutil.WriteFile(t, f.Repo, "moved.txt", "moved")
		testutil.CommitAll(t, f.Repo, "main moves on")
	}, servertest.WithEvolve(config.EvolveContributor))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootPaused, "")

	if got := f.retrosAfter(t); len(got) != 1 {
		t.Errorf("retros = %v, want one", got)
	}
}

func TestRetroHook_SubmitsWhenAutoFFMergeIsOff(t *testing.T) {
	t.Parallel()
	f := retroFixture(t, servertest.WithEvolve(config.EvolveContributor))
	servertest.SetProjectRepoNoAutoMerge(t, f.Store, "proj", f.Repo)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootCompleted, "")

	if got := f.retrosAfter(t); len(got) != 1 {
		t.Errorf("retros = %v, want one", got)
	}
}

// A merge error pauses the root, and that pause is the epic's final outcome.
func TestRetroHook_SubmitsWhenTheMergeErrors(t *testing.T) {
	t.Parallel()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(f *schedFixture, p *servertest.Turn, id string) {
		if strings.Contains(p.Address, ":one-offs/") {
			return
		}
		commitWork(t, p, id)
		// An untracked file the fast-forward would overwrite fails the merge.
		testutil.WriteFile(t, f.Repo, "01.txt", "untracked")
	}, servertest.WithEvolve(config.EvolveContributor))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.QueueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, wire.EventRootPaused, "")

	if got := f.retrosAfter(t); len(got) != 1 {
		t.Errorf("retros = %v, want one", got)
	}
}
