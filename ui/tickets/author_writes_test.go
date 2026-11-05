package tickets

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowactions"
)

// applyStatus runs the status write and returns the toast it produced, if any.
func applyStatus(t *testing.T, m Model, status schema.Status) string {
	t.Helper()
	_, cmd := m.handleTicketMenuPick(rowactions.TicketPickMsg{Target: rowactions.TicketTarget{Address: m.selectedTicketPath()}, Status: status})
	if cmd == nil {
		t.Fatal("status change issued nothing")
	}
	if msg, ok := cmd().(notify.NotifyMsg); ok {
		return msg.Message
	}
	return ""
}

func TestAuthorWrites_OpenAndDraftWriteTheStoreFileAndPingTheServer(t *testing.T) {
	var calls []string
	m, file := serverModelWithStoreTicket(t, tickettest.FakeServerAPI{Calls: &calls}, false)
	// The served File points elsewhere: the write must follow the address.
	m.sidebar.Epics[0].Tickets[0].File = filepath.Join(t.TempDir(), "stale.md")

	if toast := applyStatus(t, m, schema.StatusOpen); toast != "" || tickettest.ReadStatus(t, file) != "open" {
		t.Fatalf("open: toast %q, status %q", toast, tickettest.ReadStatus(t, file))
	}
	if toast := applyStatus(t, m, schema.StatusDraft); toast != "" || tickettest.ReadStatus(t, file) != "draft" {
		t.Fatalf("draft: toast %q, status %q", toast, tickettest.ReadStatus(t, file))
	}
	if want := []string{"changed gx:alpha/20", "changed gx:alpha/20"}; !slices.Equal(calls, want) {
		t.Errorf("calls = %v, want %v", calls, want)
	}
}

func TestAuthorWrites_WorkWithoutAServer(t *testing.T) {
	m, file := serverModelWithStoreTicket(t, tickettest.FakeServerAPI{}, false)
	m.server.api = nil
	if toast := applyStatus(t, m, schema.StatusOpen); toast != "" || tickettest.ReadStatus(t, file) != "open" {
		t.Fatalf("toast %q, status %q", toast, tickettest.ReadStatus(t, file))
	}
}

func TestAuthorWrites_DoneOnlyOnMapEpics(t *testing.T) {
	m, file := serverModelWithStoreTicket(t, tickettest.FakeServerAPI{}, false)
	if toast := applyStatus(t, m, schema.StatusDone); toast != "done is set by the server" || tickettest.ReadStatus(t, file) != "draft" {
		t.Errorf("loop epic: toast %q, status %q", toast, tickettest.ReadStatus(t, file))
	}

	m, file = serverModelWithStoreTicket(t, tickettest.FakeServerAPI{}, true)
	if toast := applyStatus(t, m, schema.StatusDone); toast != "" || tickettest.ReadStatus(t, file) != "done" {
		t.Errorf("map epic: toast %q, status %q", toast, tickettest.ReadStatus(t, file))
	}
}

func TestAuthorWrites_DoneRefusedWhileBlocked(t *testing.T) {
	m, file := serverModelWithStoreTicket(t, tickettest.FakeServerAPI{}, true)
	m.sidebar.Epics[0].Tickets = append(m.sidebar.Epics[0].Tickets, gxtickets.Ticket{Identifier: "19", Number: 19, Path: "gx:alpha/19", Status: "open"})
	m.sidebar.Epics[0].Tickets[0].BlockedBy = []string{"19"}
	if toast := applyStatus(t, m, schema.StatusDone); !strings.Contains(toast, "unresolved blocked_by") || tickettest.ReadStatus(t, file) != "draft" {
		t.Errorf("toast %q, status %q", toast, tickettest.ReadStatus(t, file))
	}
}

func TestAuthorWrites_OpenRefusedOnAnEmptyBody(t *testing.T) {
	m, file := serverModelWithStoreTicket(t, tickettest.FakeServerAPI{}, false)
	if err := os.WriteFile(file, []byte("---\nid: \"20\"\nstatus: draft\ntype: research\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if toast := applyStatus(t, m, schema.StatusOpen); !strings.Contains(toast, "empty body") || tickettest.ReadStatus(t, file) != "draft" {
		t.Errorf("toast %q, status %q", toast, tickettest.ReadStatus(t, file))
	}
}
