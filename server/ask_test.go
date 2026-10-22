package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const askBody = "## Question\n\nShould we keep the cache?\n\nIt costs memory.\n\n" +
	"```gx-question\ntype: yes-no\nyes: keep it\nno: drop it\n```\n\n## Handoff\n\nCache code is half done.\n"

func askFixture(t *testing.T, ticketType string) (*servertest.Harness, string) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{Type: ticketType, Status: "claimed"})
	servertest.SetProjectRepo(t, store, "proj", t.TempDir())
	return servertest.StartWithStore(t, store), filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
}

func askLog(t *testing.T, path string) []tickets.LogEvent {
	t.Helper()
	evs, err := tickets.ReadLog(path)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func TestAsk_AppendsQuestionAndSetsIterationStatus(t *testing.T) {
	for _, typ := range []string{"implement", "code-review", "prompt"} {
		t.Run(typ, func(t *testing.T) {
			h, path := askFixture(t, typ)
			res, err := h.Client.TicketAsk(context.Background(), "proj:epic-a/01", askBody)
			if err != nil || res.Refused {
				t.Fatalf("ask = %+v, %v", res, err)
			}
			evs := askLog(t, path)
			if len(evs) != 1 {
				t.Fatalf("log has %d events, want 1", len(evs))
			}
			ev := evs[0]
			if ev.Type != tickets.EventPauseQuestion || ev.By != "agent" || ev.Summary != "Should we keep the cache?" ||
				ev.Text != "It costs memory." || ev.Handoff != "Cache code is half done." ||
				!strings.Contains(ev.Block, "type: yes-no") {
				t.Errorf("event = %+v", ev)
			}
			tk, err := schema.ParseTicket(path)
			if err != nil {
				t.Fatal(err)
			}
			if tk.IterationStatus != schema.IterationStatusAsking {
				t.Errorf("iteration_status = %q", tk.IterationStatus)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), "iteration_status: asking") {
				t.Errorf("ticket file = %q, want iteration_status: asking written", raw)
			}
		})
	}
}

func TestAsk_Refusals(t *testing.T) {
	tests := []struct {
		name, body, address, reason string
	}{
		{"bad yaml", "## Question\n\nQ?\n\n```gx-question\ntype: [\n```\n\n## Handoff\n\nh\n", "proj:epic-a/01", server.ReasonInvalidQuestion},
		{"no question", "## Handoff\n\nh\n", "proj:epic-a/01", server.ReasonInvalidQuestion},
		{"missing handoff", "## Question\n\nQ?\n", "proj:epic-a/01", server.ReasonHandoffRequired},
		{"empty handoff", "## Question\n\nQ?\n\n## Handoff\n\n", "proj:epic-a/01", server.ReasonHandoffRequired},
		{"unknown ticket", askBody, "proj:epic-a/99", server.ReasonUnknownTicket},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, path := askFixture(t, "implement")
			res, err := h.Client.TicketAsk(context.Background(), tc.address, tc.body)
			if err != nil || !res.Refused || res.Reason != tc.reason {
				t.Fatalf("ask = %+v, %v; want refusal %s", res, err, tc.reason)
			}
			if _, err := os.Stat(tickets.LogPath(path)); !os.IsNotExist(err) {
				t.Errorf("a refused ask must not create the log, stat err = %v", err)
			}
			tk, err := schema.ParseTicket(path)
			if err != nil {
				t.Fatal(err)
			}
			if tk.IterationStatus != "" {
				t.Errorf("iteration_status = %q, want unchanged", tk.IterationStatus)
			}
		})
	}
}

func TestAsk_SecondAskRefusesWhileQuestionOpen(t *testing.T) {
	h, path := askFixture(t, "implement")
	ctx := context.Background()
	if res, err := h.Client.TicketAsk(ctx, "proj:epic-a/01", askBody); err != nil || res.Refused {
		t.Fatalf("first ask = %+v, %v", res, err)
	}
	res, err := h.Client.TicketAsk(ctx, "proj:epic-a/01", askBody)
	if err != nil || !res.Refused || res.Reason != server.ReasonQuestionOpen {
		t.Fatalf("second ask = %+v, %v; want refusal %s", res, err, server.ReasonQuestionOpen)
	}
	if n := len(askLog(t, path)); n != 1 {
		t.Errorf("log has %d events, want 1", n)
	}
}
