package server_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// trunkFixture is a sched fixture whose project's trunk is develop, a branch
// at main that nothing has checked out.
func trunkFixture(t *testing.T, opts servertest.TicketOpts, agentTurn func(f *schedFixture, p servertest.Prompt, id string)) *schedFixture {
	t.Helper()
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": opts}, agentTurn)
	f.git(t, "branch", "develop")
	servertest.SetProjectTrunk(t, f.store, "proj", f.repo, "develop")
	return f
}

func (f *schedFixture) rev(t *testing.T, ref string) string {
	t.Helper()
	return strings.TrimSpace(f.git(t, "rev-parse", ref))
}

func (f *schedFixture) runEpic(t *testing.T, wait string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	servertest.WaitForEvent(ctx, t, f.queueEpic(ctx, t, "01"), wait, "")
}

func TestTrunk_TopLevelWorkStartsFromAndMergesOnTrunk(t *testing.T) {
	f := trunkFixture(t, servertest.TicketOpts{}, func(_ *schedFixture, p servertest.Prompt, id string) { commitWork(t, p, id) })
	main := f.rev(t, "main")
	develop := f.rev(t, "develop")

	f.runEpic(t, server.EventRootCompleted)

	if body := readFile(t, f.ticketFile(t, "01")); !strings.Contains(body, "resolved_base: develop@"+develop) {
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
	f := trunkFixture(t, servertest.TicketOpts{Base: "release-1.2"}, func(_ *schedFixture, p servertest.Prompt, id string) { commitWork(t, p, id) })
	f.git(t, "branch", "release-1.2")
	release := f.rev(t, "release-1.2")

	f.runEpic(t, server.EventRootCompleted)

	if body := readFile(t, f.ticketFile(t, "01")); !strings.Contains(body, "resolved_base: release-1.2@"+release) {
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
	f := trunkFixture(t, servertest.TicketOpts{}, func(f *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
		f.git(t, "branch", "next", "develop")
		servertest.SetProjectTrunk(t, f.store, "proj", f.repo, "next")
	})
	develop := f.rev(t, "develop")

	f.runEpic(t, server.EventRootCompleted)

	if got := f.rev(t, "next"); got != f.rev(t, schedEpic) {
		t.Errorf("next at %s, want the feature branch tip", got)
	}
	if got := f.rev(t, "develop"); got != develop {
		t.Errorf("old trunk develop moved to %s", got)
	}
}

func TestTrunk_DivergedNewTrunkNeedsRebase(t *testing.T) {
	f := trunkFixture(t, servertest.TicketOpts{}, func(f *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
		// Past the cap, the non-ff pauses the root rather than forking a merge ticket.
		for _, mid := range []string{"02", "03"} {
			servertest.WriteTicketWith(t, f.store, "proj", schedEpic, mid, "t", servertest.TicketOpts{Type: "merge", Status: "done"})
		}
		f.git(t, "checkout", "-q", "-b", "next", "develop")
		testutil.WriteFile(t, f.repo, "next.txt", "next")
		testutil.CommitAll(t, f.repo, "next moves on")
		f.git(t, "checkout", "-q", "main")
		servertest.SetProjectTrunk(t, f.store, "proj", f.repo, "next")
	})

	f.runEpic(t, server.EventRootPaused)

	if got := strings.TrimSpace(f.git(t, "log", "-1", "--format=%s", "next")); got != "next moves on" {
		t.Errorf("next head = %q, want it untouched", got)
	}
}
