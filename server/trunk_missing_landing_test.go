package server_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// trunkDeletedMidRun runs the one-ticket epic with trunk develop deleted while
// the agent works, and waits for the landing to park.
func trunkDeletedMidRun(t *testing.T, ctx context.Context) (f *schedFixture, evs <-chan server.Event, develop string) {
	t.Helper()
	f = trunkFixture(t, servertest.TicketOpts{}, func(f *schedFixture, p servertest.Prompt, id string) {
		commitWork(t, p, id)
		f.git(t, "branch", "-D", "develop")
	})
	develop = f.rev(t, "develop")
	evs = f.queueEpic(ctx, t, "01")
	servertest.WaitForEvent(ctx, t, evs, server.EventIterationParked, "")
	return f, evs, develop
}

func TestTrunkMissingAtLanding_ParksTheTicketAndLandsNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f, _, develop := trunkDeletedMidRun(t, ctx)

	body := readFile(t, f.ticketFile(t, "01"))
	for _, want := range []string{"status: needs-repair", "park_kind: trunk-missing", "## Needs Repair", `trunk "develop" from project.json is not a local branch`} {
		if !strings.Contains(body, want) {
			t.Errorf("ticket lacks %q:\n%s", want, body)
		}
	}
	if out := f.git(t, "log", "--all", "--format=%s", "--not", "--glob=refs/heads/ralph-loop/*"); strings.Contains(out, "work 01") {
		t.Errorf("work landed outside its iteration branch:\n%s", out)
	}
	if got := f.rev(t, "main"); got != develop {
		t.Errorf("main moved to %s", got)
	}
}

func TestTrunkMissingAtLanding_LandAfterTheFixLandsOnTrunk(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f, evs, develop := trunkDeletedMidRun(t, ctx)

	res, err := f.h.Client.Repair(ctx, "land", server.RepairRequest{Address: "proj:" + schedEpic + "/01"})
	if err != nil || !res.Refused || !strings.Contains(res.Message, `trunk "develop"`) {
		t.Fatalf("land with trunk still missing = %+v, %v; want a refusal naming the trunk", res, err)
	}

	f.git(t, "branch", "develop", develop)
	res, err = f.h.Client.Repair(ctx, "land", server.RepairRequest{Address: "proj:" + schedEpic + "/01"})
	if err != nil || res.Refused {
		t.Fatalf("land: %+v, %v", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	if got := f.git(t, "show", "develop:01.txt"); got != "01" {
		t.Errorf("develop:01.txt = %q, want the ticket's work", got)
	}
}
