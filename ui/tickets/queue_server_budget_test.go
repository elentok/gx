package tickets

import (
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
)

func loadedServerQueueWithBudget(t *testing.T, budget wire.BudgetStatus) QueueModel {
	t.Helper()
	m, _ := deliverQueue(t, wire.Snapshot{Seq: 1, Budget: budget, Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "open"},
	}}, []wire.QueueItem{{Address: "gx:alpha/01"}})
	return m
}

func TestQueueServerMode_HeaderShowsTodaysSpendOfLimit(t *testing.T) {
	m := loadedServerQueueWithBudget(t, wire.BudgetStatus{Total: 12.5, SoftLimit: 50})

	if got, want := m.header().queueHeaderTitle(), "Queue · today $12.50 of $50.00"; got != want {
		t.Errorf("queueHeaderTitle() = %q, want %q", got, want)
	}
}

func TestQueueServerMode_HeaderColourBands(t *testing.T) {
	cases := []struct {
		name  string
		total float64
		want  string
	}{
		{"below 80%", 39, "default"},
		{"from 80%", 40, "warning"},
		{"at the limit", 50, "alarm"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := budgetTotalStyle(tc.total, 50)
			var name string
			switch got.GetForeground() {
			case epicStatusPausedRepairStyle.GetForeground():
				name = "alarm"
			case epicStatusProblemStyle.GetForeground():
				name = "warning"
			default:
				name = "default"
			}
			if name != tc.want {
				t.Errorf("total %v: band = %s, want %s", tc.total, name, tc.want)
			}
		})
	}
}

func TestQueueServerMode_EpicStatusLineShowsTicketCostSum(t *testing.T) {
	m, _ := deliverQueue(t, wire.Snapshot{Seq: 1,
		Budget: wire.BudgetStatus{Total: 5, SoftLimit: 50, Roots: map[string]float64{"gx:alpha": 99}},
		Tickets: []wire.TicketInfo{
			{Address: "gx:alpha/01", Title: "First", Status: "done", ActualCost: 1.5},
			{Address: "gx:alpha/02", Title: "Second", Status: "open", ActualCost: 1.75},
		}}, []wire.QueueItem{{Address: "gx:alpha/01"}, {Address: "gx:alpha/02"}})

	line := epicStatusLineText(t, m)
	if !strings.Contains(line, "$3.25") || strings.Contains(line, "$99") {
		t.Errorf("Status line should show the ticket sum $3.25, not the ledger root: %q", line)
	}
}
