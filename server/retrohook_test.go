package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets"
)

func withEvolve(mode string) func(*server.Config) {
	return func(c *server.Config) { c.Evolve = mode }
}

// completeEpic runs the fixture's epic to completion and returns the retro
// one-off epics the hook submitted in the project by then.
func (f *schedFixture) completeEpic(t *testing.T) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs := f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")
	retros, err := filepath.Glob(filepath.Join(f.store, "proj", "evolve-"+schedEpic+"-*"))
	if err != nil {
		t.Fatal(err)
	}
	return retros
}

func retroFixture(t *testing.T, opts ...func(*server.Config)) *schedFixture {
	t.Helper()
	return newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p servertest.Prompt, id string) {
		// A submitted retro launches too; it is commitless, so it commits nothing.
		if strings.Contains(p.Address, "evolve-") {
			return
		}
		commitWork(t, p, id)
	}, opts...)
}

func TestRetroHook_SubmitsWhenAutoMergeIsOff(t *testing.T) {
	f := retroFixture(t, withEvolve(config.EvolveContributor), func(c *server.Config) { c.AutoMergeEpic = false })
	if got := f.completeEpic(t); len(got) != 1 {
		t.Errorf("retros = %v, want one", got)
	}
}

func TestRetroHook_SubmitsAfterTheEpicLands(t *testing.T) {
	f := retroFixture(t, withEvolve(config.EvolveContributor))
	if got := f.completeEpic(t); len(got) != 1 {
		t.Errorf("retros = %v, want one", got)
	}
}

func TestRetroHook_NothingWhenEvolveIsOff(t *testing.T) {
	f := retroFixture(t, withEvolve(config.EvolveOff))
	if got := f.completeEpic(t); len(got) != 0 {
		t.Errorf("retros = %v, want none", got)
	}
}

func TestRetroHook_NothingWhenTheProjectOptedOut(t *testing.T) {
	f := retroFixture(t, withEvolve(config.EvolveContributor))
	dir := filepath.Join(f.store, "proj")
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
	f := retroFixture(t, withEvolve(config.EvolveContributor))
	if err := tickets.StampEpicFields(filepath.Join(f.store, "proj", schedEpic), map[string]any{"evolved_at": time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if got := f.completeEpic(t); len(got) != 0 {
		t.Errorf("retros = %v, want none", got)
	}
}

func TestRetroHook_MaintainerWithoutAGxProjectSubmitsNothing(t *testing.T) {
	f := retroFixture(t, withEvolve(config.EvolveMaintainer))
	if got := f.completeEpic(t); len(got) != 0 {
		t.Errorf("retros = %v, want none", got)
	}
}

func TestRetroHook_MaintainerWithAGxProjectSubmits(t *testing.T) {
	f := retroFixture(t, withEvolve(config.EvolveMaintainer))
	if err := os.MkdirAll(filepath.Join(f.store, "gx"), 0o755); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, f.store, "gx", f.repo)
	if got := f.completeEpic(t); len(got) != 1 {
		t.Errorf("retros = %v, want one", got)
	}
}
