package server_test

import (
	"github.com/elentok/gx/server/wire"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// trunkFixture is a sched fixture whose project's trunk is develop, a branch
// at main that nothing has checked out.
func trunkFixture(t *testing.T, opts servertest.TicketOpts, agentTurn func(f *schedFixture, p *servertest.Turn, id string)) *schedFixture {
	t.Helper()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": opts}, agentTurn)
	f.Git(t, "branch", "develop")
	servertest.SetProjectTrunk(t, f.Store, "proj", f.Repo, "develop")
	return f
}

func (f *schedFixture) rev(t *testing.T, ref string) string {
	t.Helper()
	return strings.TrimSpace(f.Git(t, "rev-parse", ref))
}

func (f *schedFixture) runEpic(t *testing.T, wait string) {
	t.Helper()
	f.RunEpic(t, wait)
}

func TestTrunk_TopLevelWorkStartsFromAndMergesOnTrunk(t *testing.T) {
	t.Parallel()
	f := trunkFixture(t, servertest.TicketOpts{}, func(_ *schedFixture, p *servertest.Turn, id string) { commitWork(t, p, id) })
	main := f.rev(t, "main")
	develop := f.rev(t, "develop")

	f.runEpic(t, wire.EventRootCompleted)

	if body := servertest.ReadFile(t, f.TicketFile(t, "01")); !strings.Contains(body, "resolved_base: develop@"+develop) {
		t.Errorf("ticket lacks resolved_base develop@%s:\n%s", develop, body)
	}
	if got := f.rev(t, "develop"); got != f.rev(t, schedEpic) {
		t.Errorf("develop at %s, want the feature branch tip", got)
	}
	if got := f.rev(t, "main"); got != main {
		t.Errorf("main moved to %s", got)
	}
}

func TestTrunk_ExplicitBaseMovesOnlyTheStart(t *testing.T) {
	t.Parallel()
	f := trunkFixture(t, servertest.TicketOpts{Base: "release-1.2"}, func(_ *schedFixture, p *servertest.Turn, id string) { commitWork(t, p, id) })
	f.Git(t, "branch", "release-1.2")
	release := f.rev(t, "release-1.2")

	f.runEpic(t, wire.EventRootCompleted)

	if body := servertest.ReadFile(t, f.TicketFile(t, "01")); !strings.Contains(body, "resolved_base: release-1.2@"+release) {
		t.Errorf("ticket lacks resolved_base release-1.2@%s:\n%s", release, body)
	}
	if got := f.rev(t, "develop"); got != f.rev(t, schedEpic) {
		t.Errorf("develop at %s, want the feature branch tip", got)
	}
	if got := f.rev(t, "release-1.2"); got != release {
		t.Errorf("release-1.2 moved to %s", got)
	}
}

func TestTrunk_EditedBetweenClaimAndMergingMergesOnTheNewTrunk(t *testing.T) {
	t.Parallel()
	f := trunkFixture(t, servertest.TicketOpts{}, func(f *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
		f.Git(t, "branch", "next", "develop")
		servertest.SetProjectTrunk(t, f.Store, "proj", f.Repo, "next")
	})
	develop := f.rev(t, "develop")

	f.runEpic(t, wire.EventRootCompleted)

	if got := f.rev(t, "next"); got != f.rev(t, schedEpic) {
		t.Errorf("next at %s, want the feature branch tip", got)
	}
	if got := f.rev(t, "develop"); got != develop {
		t.Errorf("old trunk develop moved to %s", got)
	}
}

func TestTrunk_DivergedNewTrunkNeedsRebase(t *testing.T) {
	t.Parallel()
	f := trunkFixture(t, servertest.TicketOpts{}, func(f *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
		// Past the cap, the non-ff pauses the root rather than forking a merge ticket.
		for _, mid := range []string{"02", "03", "04"} {
			servertest.WriteTicketWith(t, f.Store, "proj", schedEpic, mid, "t", servertest.TicketOpts{Type: "merge", Status: "done"})
		}
		f.Git(t, "checkout", "-q", "-b", "next", "develop")
		testutil.WriteFile(t, f.Repo, "next.txt", "next")
		testutil.CommitAll(t, f.Repo, "next moves on")
		f.Git(t, "checkout", "-q", "main")
		servertest.SetProjectTrunk(t, f.Store, "proj", f.Repo, "next")
	})

	f.runEpic(t, wire.EventRootPaused)

	if got := strings.TrimSpace(f.Git(t, "log", "-1", "--format=%s", "next")); got != "next moves on" {
		t.Errorf("next head = %q, want it untouched", got)
	}
}
