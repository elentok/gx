package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui/nav"
	ticketsui "github.com/elentok/gx/ui/tickets"
)

func queueView(h *harness) string {
	h.t.Helper()
	q, ok := h.m.activePage().model.(ticketsui.QueueModel)
	if !ok {
		h.t.Fatalf("active page is %T, want the Queue tab", h.m.activePage().model)
	}
	return ansi.Strip(q.View().Content)
}

func queuedTicket(status string, claimedAt time.Time) []server.TicketInfo {
	return []server.TicketInfo{{Address: "gx:alpha/01", Title: "Zeta ticket", Status: status, ClaimedAt: claimedAt}}
}

func openQueueTab(h *harness) {
	h.t.Helper()
	h.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	h.probeUp()
	h.switchTo(nav.TabQueue)
	h.pump()
}

func TestQueueStream_UpdatesOnEventWhileOpen(t *testing.T) {
	h, f := newStreamHarness(t)
	f.tickets = queuedTicket("open", time.Time{})
	f.queue = []server.QueueItem{{Address: "gx:alpha/01"}}
	openQueueTab(h)
	if view := queueView(h); !strings.Contains(view, "Zeta ticket") || strings.Contains(view, "implementing") {
		t.Fatalf("idle queue:\n%s", view)
	}

	f.mu.Lock()
	f.tickets = queuedTicket("claimed", time.Now())
	f.mu.Unlock()
	f.stream(0).ch <- server.Event{Seq: 11, Type: server.EventTicketClaimed, Address: "gx:alpha/01"}
	h.pump()

	if view := queueView(h); !strings.Contains(view, "implementing") {
		t.Errorf("claim did not show without R:\n%s", view)
	}
}

func TestQueueStream_ClaimWhileHiddenShowsRunningOnOpen(t *testing.T) {
	h, f := newStreamHarness(t)
	f.tickets = queuedTicket("open", time.Time{})
	f.queue = []server.QueueItem{{Address: "gx:alpha/01"}}
	openQueueTab(h)
	h.switchTo(nav.TabWorktrees)

	f.mu.Lock()
	f.tickets = queuedTicket("claimed", time.Now())
	f.mu.Unlock()
	f.stream(0).ch <- server.Event{Seq: 11, Type: server.EventTicketClaimed, Address: "gx:alpha/01"}
	h.pump()
	h.switchTo(nav.TabQueue)
	h.pump()

	if view := queueView(h); !strings.Contains(view, "implementing") {
		t.Errorf("claim made while hidden is not running on open:\n%s", view)
	}
}

func TestQueueStream_ModeChangeFromAnotherClientShows(t *testing.T) {
	h, f := newStreamHarness(t)
	f.tickets = queuedTicket("open", time.Time{})
	f.queue = []server.QueueItem{{Address: "gx:alpha/01"}}
	openQueueTab(h)

	f.mu.Lock()
	f.mode = server.ModePaused
	f.mu.Unlock()
	f.stream(0).ch <- server.Event{Seq: 11, Type: server.EventQueueChanged}
	h.pump()

	// The pause key offers resume only if the tab knows the queue is paused.
	h.send(tea.KeyPressMsg{Code: 'p', Text: "p"})
	h.pump()
	if view := queueView(h); !strings.Contains(view, "Resume the queue?") {
		t.Errorf("pause key does not follow the delivered mode:\n%s", view)
	}
}

func TestQueueStream_IdleTUIFetchesNothing(t *testing.T) {
	h, f := newStreamHarness(t)
	f.tickets = queuedTicket("open", time.Time{})
	openQueueTab(h)
	snapshots, queueGets := f.fetches()

	h.pump()
	h.switchTo(nav.TabWorktrees)
	h.switchTo(nav.TabQueue)
	h.pump()

	if s, q := f.fetches(); s != snapshots || q != queueGets {
		t.Errorf("idle fetches grew: snapshots %d→%d, queue reads %d→%d", snapshots, s, queueGets, q)
	}
}
