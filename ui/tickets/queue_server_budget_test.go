package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func loadedServerQueueWithBudget(t *testing.T, budget server.BudgetStatus) QueueModel {
	t.Helper()
	api := fakeServerAPI{
		snap: server.Snapshot{Seq: 1, Budget: budget, Tickets: []server.TicketInfo{
			{Address: "gx:alpha/01", Title: "First", Status: "open"},
		}},
		queue: []server.QueueItem{{Address: "gx:alpha/01"}},
	}
	m := NewQueueModel(t.TempDir(), ui.Settings{}, nil, keys.New(nil)).WithServerLink(api, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	next, _ = next.(QueueModel).Update(m.cmdLoadQueue()())
	return next.(QueueModel)
}

func TestQueueServerMode_HeaderShowsTodaysSpendOfLimit(t *testing.T) {
	m := loadedServerQueueWithBudget(t, server.BudgetStatus{Total: 12.5, SoftLimit: 50})

	if got, want := m.queueHeaderTitle(), "Queue · today $12.50 of $50.00"; got != want {
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
			case epicStatusParkedRepairStyle.GetForeground():
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

func TestQueueServerMode_RootRowShowsItsCostToday(t *testing.T) {
	m := loadedServerQueueWithBudget(t, server.BudgetStatus{
		Total: 5, SoftLimit: 50, Roots: map[string]float64{"gx:alpha": 3.25},
	})

	if view := m.View().Content; !strings.Contains(view, "$3.25") {
		t.Errorf("queue view does not show the root's $3.25:\n%s", view)
	}
}
