package budget

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

type noopNotifier struct{}

func (noopNotifier) Notify(context.Context, osnotify.Notification) error { return nil }

// followUpHost records the follow-ups the budget files.
type followUpHost struct {
	Host
	addrs []tickets.Address
	files []FollowUp
}

func (h *followUpHost) WriteFollowUp(from tickets.Address, f FollowUp) (string, error) {
	h.addrs = append(h.addrs, from)
	h.files = append(h.files, f)
	return "", nil
}

func TestAlertLongTicket_MarksTheTicketAndFilesTheSameKeyedFollowUpEachTime(t *testing.T) {
	t.Parallel()
	epicPath := filepath.Join(t.TempDir(), "epic-a")
	if err := os.MkdirAll(filepath.Join(epicPath, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, epicPath)
	path := filepath.Join(epicPath, "issues", "01-a.md")
	out, err := schema.MarshalTicket(schema.Ticket{ID: "01", Status: schema.StatusClaimed, Type: schema.TypeImplement}, "\n# 01 — a\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}

	events := broker.New(0)
	var h settings.Holder
	h.Init(settings.Options{Notifier: noopNotifier{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Events: events})
	host := &followUpHost{}
	b := &Budget{
		settings: &h, events: events, host: host,
		chat: notify.NewServerChat(notify.ServerChatConfig{}),
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	first := tickets.Address{Project: "proj", Epic: "epic-a", ID: "01"}
	second := tickets.Address{Project: "proj", Epic: "epic-b", ID: "01"}
	b.alertLongTicket(first, path, 3)
	b.alertLongTicket(second, path, 7)

	if len(host.files) != 2 || host.addrs[0] != first || host.addrs[1] != second {
		t.Fatalf("follow-ups = %+v from %v, want one per alert from each ticket", host.files, host.addrs)
	}
	for _, f := range host.files {
		if f.Key != longTicketKey || f.Slug != longTicketKey {
			t.Errorf("follow-up key/slug = %q/%q, want %q", f.Key, f.Slug, longTicketKey)
		}
	}
	tk, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatal(err)
	}
	if tk.LongTicketTurns != 7 {
		t.Errorf("LongTicketTurns = %d, want the last alert's 7", tk.LongTicketTurns)
	}
	if err := ClearLongTicketMark(path); err != nil {
		t.Fatal(err)
	}
	if tk, _ = schema.ParseTicket(path); tk.LongTicketTurns != 0 {
		t.Errorf("after clear LongTicketTurns = %d, want 0", tk.LongTicketTurns)
	}
}
