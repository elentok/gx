package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server"
	gxtickets "github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/viewmodel"
)

// olderServerState is a snapshot from a server that predates rendered status.
func olderServerState() viewmodel.State {
	return viewmodel.State{}.ApplySnapshot(server.Snapshot{Seq: 1, Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
	}}).SetQueue([]server.QueueItem{{Address: "gx:alpha/01"}})
}

func TestOlderServer_TicketsTabShowsNoticeAndNoTickets(t *testing.T) {
	m := newServerModel(t)
	st := olderServerState()
	next, _ := m.WithServerState(&st, ServerLinkUp)
	next, _ = next.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	view := ansi.Strip(next.View().Content)
	if !strings.Contains(view, olderServerNotice) || strings.Contains(view, "First") {
		t.Fatalf("view = %s", view)
	}
	if got := len(next.(Model).epics); got != 0 {
		t.Fatalf("epics = %d, want none (no ticket data for menu, preview or search)", got)
	}
}

func TestOlderServer_QueueTabShowsNoticeAndNoTickets(t *testing.T) {
	m := newServerQueueModel(t, nil)
	st := olderServerState()
	next, _ := m.WithServerState(&st, ServerLinkUp)
	view := ansi.Strip(next.View().Content)
	if !strings.Contains(view, olderServerNotice) || strings.Contains(view, "First") {
		t.Fatalf("view = %s", view)
	}
	if got := len(next.(QueueModel).epics); got != 0 {
		t.Fatalf("epics = %d, want none", got)
	}
}

func TestOlderServer_CurrentServerClearsNotice(t *testing.T) {
	m := newServerModel(t)
	st := olderServerState()
	next, _ := m.WithServerState(&st, ServerLinkUp)
	current := applySnapshot(viewmodel.State{}, server.Snapshot{Seq: 2, Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
	}})
	next, _ = next.(Model).WithServerState(&current, ServerLinkUp)
	next, _ = next.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if view := ansi.Strip(next.View().Content); strings.Contains(view, olderServerNotice) {
		t.Fatalf("notice stays after a current server's snapshot:\n%s", view)
	}
}

func TestNoServer_QueueSaysServerNotRunning(t *testing.T) {
	m := newServerQueueModel(t, nil)
	next, _ := m.WithServerState(nil, ServerLinkDown)
	if view := ansi.Strip(next.View().Content); !strings.Contains(view, "server not running") {
		t.Fatalf("view = %s", view)
	}
}

// With no server the Tickets and Archived tabs show the file's own status: the
// blocked overlay is the server's to compute.
func TestNoServer_TicketsShowRawStatus(t *testing.T) {
	blocker := gxtickets.Ticket{Number: 1, Identifier: "01", Path: "p:e/01", Status: "open"}
	blocked := gxtickets.Ticket{Number: 2, Identifier: "02", Path: "p:e/02", Status: "open", BlockedBy: []string{"01"}}
	epic := gxtickets.Epic{Name: "p:e", Tickets: []gxtickets.Ticket{blocker, blocked}}

	if epic.RenderedStatus(blocked) != gxtickets.StatusBlocked {
		t.Fatal("the test needs the computed status to differ from the raw one")
	}
	if got := ticketStatus(epic, blocked); got != gxtickets.StatusOpen {
		t.Errorf("ticketStatus = %v, want open", got)
	}

	m, _ := withLink(NewModel(t.TempDir(), ui.Settings{}, keys.Manager{}), ServerLinkDown)
	if !m.readsDisk() || m.olderServer {
		t.Fatal("a disk-reading tab must not show the older-server notice")
	}
}
