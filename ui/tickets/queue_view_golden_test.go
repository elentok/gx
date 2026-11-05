package tickets

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

var goldenNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func TestQueueViewGolden_TwoEpicsWithHeaderLines(t *testing.T) {
	snap := wire.Snapshot{Seq: 1, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "done", ElapsedTime: 754, ActualCost: 1.5},
		{Address: "gx:alpha/02", Title: "Second", Status: "open"},
		{Address: "gx:beta/01", Title: "Other", Status: "open"},
	}}
	m := goldenQueueState(goldenQueue(t), snap, []wire.QueueItem{{Address: "gx:alpha/02"}, {Address: "gx:beta/01"}}, goldenNow)
	m.server.serverBudget =wire.BudgetStatus{Total: 12.5, SoftLimit: 50}
	assertGolden(t, "queue_two_epics_header", m.View().Content)
}

func TestQueueViewGolden_LiveTicketRow(t *testing.T) {
	claimedAt := time.Now().Add(-(2*time.Hour + 30*time.Second))
	snap := wire.Snapshot{Seq: 1, Tickets: goldenSnapshotTickets("claimed", claimedAt)}
	m := goldenQueueState(goldenQueue(t), snap, []wire.QueueItem{{Address: "gx:alpha/02"}, {Address: "gx:alpha/03"}}, goldenNow)
	m = setGoldenLive(m, "gx:alpha", "02", 45_200)
	assertGolden(t, "queue_live_row", m.View().Content)
}

func TestQueueViewGolden_ServerDown(t *testing.T) {
	m := goldenQueue(t)
	next, _ := m.WithServerState(nil, tabapi.ServerLinkDown)
	assertGolden(t, "queue_server_down", next.View().Content)
}
