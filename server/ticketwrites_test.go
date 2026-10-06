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
	"github.com/elentok/gx/testutil"
)

func startTicketWrites(t *testing.T) (*servertest.Harness, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	return h, filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
}

func TestPark_RefusesWithoutReasonAndWritesNothing(t *testing.T) {
	h, path := startTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "  ")
	if err != nil || !res.Refused || res.Reason != server.ReasonReasonRequired {
		t.Fatalf("park = %+v, %v; want %s refusal", res, err, server.ReasonReasonRequired)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "needs-repair") {
		t.Errorf("refused park changed the ticket:\n%s", data)
	}
}

func TestPark_WritesStatusSectionAndOneEvent(t *testing.T) {
	h, path := startTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "needs a human look")
	if err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"status: needs-repair", "## Needs Repair", "needs a human look"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("ticket lacks %q:\n%s", want, data)
		}
	}
	hist, err := h.Client.History(ctx, "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Events) != 1 || hist.Events[0].Type != "needs-repair" || hist.Events[0].Kind != "manual-park" {
		t.Errorf("events = %+v, want one needs-repair/manual-park", hist.Events)
	}
}

func TestPark_RefusesUnknownTicket(t *testing.T) {
	h, _ := startTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketPark(ctx, "proj:epic-a/99", "why")
	if err != nil || !res.Refused || res.Reason != server.ReasonUnknownTicket {
		t.Fatalf("park = %+v, %v", res, err)
	}
}

func TestRelaunch_StartsAFreshIterationOfAParkedTicket(t *testing.T) {
	h, path := startTicketWrites(t)
	start, prompt, _ := registerLaunch(h)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	res, err := h.Client.TicketRelaunch(ctx, "proj:epic-a/01")
	if err != nil || res.Refused {
		t.Fatalf("relaunch = %+v, %v", res, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "status: claimed") {
		t.Errorf("relaunched ticket is not claimed:\n%s", data)
	}
	if len(*start) == 0 || !strings.Contains(strings.Join(*prompt, " "), "proj:epic-a/01") {
		t.Errorf("agent start = %v, prompt = %v", *start, *prompt)
	}
}
