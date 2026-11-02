package server

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

type noopNotifier struct{}

func (noopNotifier) Notify(context.Context, osnotify.Notification) error { return nil }

func TestAlertLongTicket_ASecondLongTicketAddsSeenAgainNotANewDraft(t *testing.T) {
	t.Parallel()
	s, store, _, path := mergedInvestigation(t, investigateReport(proposalHumanOnly), "proj:follow-ups")
	s.cfg.Notifier = noopNotifier{}
	s.events = newBroker(0)
	s.chat = ralphloop.NewServerChat(ralphloop.ServerChatConfig{})

	s.alertLongTicket(tickets.Address{Project: "proj", Epic: "epic-a", ID: "01"}, path, 3)
	s.alertLongTicket(tickets.Address{Project: "proj", Epic: "epic-b", ID: "01"}, path, 3)

	got := followUpTickets(t, store)
	if len(got) != 1 {
		t.Fatalf("follow-up drafts = %d, want 1", len(got))
	}
	raw, err := os.ReadFile(got[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), "Seen again in proj:epic-b/01"); n != 1 {
		t.Errorf("seen-again lines naming epic-b/01 = %d, want 1:\n%s", n, raw)
	}
	if n := strings.Count(string(raw), "Seen again"); n != 1 {
		t.Errorf("seen-again lines = %d, want 1:\n%s", n, raw)
	}
}
