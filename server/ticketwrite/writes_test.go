package ticketwrite_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func TestPause_RefusesWithoutReasonAndWritesNothing(t *testing.T) {
	t.Parallel()
	h, path := servertest.StartTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketPause(ctx, "proj:epic-a/01", "  ")
	if err != nil || !res.Refused || res.Reason != wire.ReasonReasonRequired {
		t.Fatalf("pause = %+v, %v; want %s refusal", res, err, wire.ReasonReasonRequired)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "status: paused") {
		t.Errorf("refused pause changed the ticket:\n%s", data)
	}
}

func TestPause_WritesStatusSectionAndOneEvent(t *testing.T) {
	t.Parallel()
	h, path := servertest.StartTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketPause(ctx, "proj:epic-a/01", "needs a human look")
	if err != nil || res.Refused {
		t.Fatalf("pause = %+v, %v", res, err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"status: paused", "pause_reason: manual", "## Paused", "needs a human look"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("ticket lacks %q:\n%s", want, data)
		}
	}
	hist, err := h.Client.History(ctx, "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Events) != 1 || hist.Events[0].Type != "paused" || hist.Events[0].Kind != "manual" {
		t.Errorf("events = %+v, want one stuck/manual-pause", hist.Events)
	}
}

func TestPause_RefusesUnknownTicket(t *testing.T) {
	t.Parallel()
	h, _ := servertest.StartTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.Client.TicketPause(ctx, "proj:epic-a/99", "why")
	if err != nil || !res.Refused || res.Reason != wire.ReasonUnknownTicket {
		t.Fatalf("pause = %+v, %v", res, err)
	}
}

func TestPause_RefusesARunningDoneAlreadyPausedOrMapEpicTicket(t *testing.T) {
	t.Parallel()
	h, path := servertest.StartTicketWrites(t)
	store := h.TicketStore
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "02", "finished", servertest.TicketOpts{Status: "done"})
	servertest.WriteTicket(t, store, "proj", "map-epic", "01", "mapped", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map-epic", "ticket.md"), []byte("---\nkind: map\nstatus: open\n---\n# Map\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.Server.Registry().PutRun("proj:epic-a", runs.Run{Address: "proj:epic-a/01", Runner: "herdr", Session: agentrunner.Session{ID: "p1"}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, c := range []struct{ name, address, reason string }{
		{"running", "proj:epic-a/01", wire.ReasonIterationRunning},
		{"done", "proj:epic-a/02", wire.ReasonTicketDone},
		{"map epic", "proj:map-epic/01", wire.ReasonMapEpic},
	} {
		res, err := h.Client.TicketPause(ctx, c.address, "why")
		if err != nil || !res.Refused || res.Reason != c.reason {
			t.Errorf("pause %s = %+v, %v; want %s refusal", c.name, res, err, c.reason)
		}
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "status: paused") {
		t.Errorf("refused pause changed the running ticket:\n%s", data)
	}
}

func TestPause_RefusesAnAlreadyPausedTicket(t *testing.T) {
	t.Parallel()
	h, _ := servertest.StartTicketWrites(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketPause(ctx, "proj:epic-a/01", "first"); err != nil || res.Refused {
		t.Fatalf("pause = %+v, %v", res, err)
	}
	res, err := h.Client.TicketPause(ctx, "proj:epic-a/01", "second")
	if err != nil || !res.Refused || res.Reason != wire.ReasonAlreadyPaused {
		t.Fatalf("second pause = %+v, %v; want %s refusal", res, err, wire.ReasonAlreadyPaused)
	}
}

func TestRelaunch_StartsAFreshIterationOfAPausedTicket(t *testing.T) {
	t.Parallel()
	h, path := servertest.StartTicketWrites(t)
	start, prompt, _ := servertest.RegisterTurn(h)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketPause(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("pause = %+v, %v", res, err)
	}
	res, err := h.Client.TicketRelaunch(ctx, "proj:epic-a/01", false)
	if err != nil || res.Refused {
		t.Fatalf("relaunch = %+v, %v", res, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "status: claimed") {
		t.Errorf("relaunched ticket is not claimed:\n%s", data)
	}
	if len(start()) == 0 || !strings.Contains(strings.Join(prompt(), " "), "proj:epic-a/01") {
		t.Errorf("agent start = %v, prompt = %v", start(), prompt())
	}
}
