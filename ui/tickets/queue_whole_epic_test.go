package tickets

import (
	"slices"
	"testing"

	"github.com/elentok/gx/server"
)

func queueRowPaths(m QueueModel) []string {
	var paths []string
	for _, e := range m.buildQueueEntries() {
		if e.Value.kind == nodeQueueTicket {
			paths = append(paths, e.Value.ticket.ticket.Path)
		}
	}
	return paths
}

// An epic with one queued item and a parentless ticket added afterwards: the
// server schedules that ticket too, so the tab must show it.
func wholeEpicSnapshot(extraStatus string) server.Snapshot {
	return server.Snapshot{Seq: 1, Tickets: []server.TicketInfo{
		{Address: "gx:alpha/01", Title: "Queued", Status: "open"},
		{Address: "gx:alpha/02", Title: "Added later", Status: extraStatus},
		{Address: "gx:beta/01", Title: "Unqueued epic", Status: "open"},
	}}
}

func TestQueueWholeEpic_NonItemTicketOfQueuedEpicRenders(t *testing.T) {
	m, _ := deliverQueue(t, wholeEpicSnapshot("open"), []server.QueueItem{{Address: "gx:alpha/01"}})

	got := queueRowPaths(m)
	if !slices.Equal(got, []string{"gx:alpha/01", "gx:alpha/02"}) {
		t.Errorf("rows = %v, want both alpha tickets and nothing from beta", got)
	}
}

func TestQueueWholeEpic_EpicWithNoQueuedItemRendersNothing(t *testing.T) {
	m, _ := deliverQueue(t, wholeEpicSnapshot("open"), nil)

	if got := queueRowPaths(m); len(got) != 0 {
		t.Errorf("rows = %v, want none", got)
	}
}

func TestQueueWholeEpic_HideCompleteHidesDoneExtraTicket(t *testing.T) {
	m, _ := deliverQueue(t, wholeEpicSnapshot("done"), []server.QueueItem{{Address: "gx:alpha/01"}})
	m.hideComplete = true

	got := queueRowPaths(m)
	if !slices.Equal(got, []string{"gx:alpha/01"}) {
		t.Errorf("rows = %v, want only the queued ticket", got)
	}
}

func TestQueueWholeEpic_DeleteIsNotOfferedOnNonItemRow(t *testing.T) {
	m, _ := deliverQueue(t, wholeEpicSnapshot("open"), []server.QueueItem{{Address: "gx:alpha/01"}})
	for i, e := range m.queueTree.Entries() {
		if e.Value.kind == nodeQueueTicket && e.Value.ticket.ticket.Path == "gx:alpha/02" {
			m.queueTree.SetSelectedIndex(i)
		}
	}

	next, _ := m.handleServerDeleteKey()
	if next.(QueueModel).confirm.IsOpen {
		t.Error("delete opened a confirm on a ticket that is not a queue item")
	}
}
