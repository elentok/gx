package header

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tickets/rowfmt"
)

func testHeader(now time.Time) Header {
	return Header{Icons: ui.Icons(false), Now: func() time.Time { return now }}
}

func TestQueueHeaderPausedLineFollowsRunningCount(t *testing.T) {
	t.Parallel()
	h := testHeader(time.Now())
	h.Paused = true
	cases := []struct {
		active []wire.SummaryTicket
		want   string
	}{
		{nil, "Paused. No tickets running."},
		{[]wire.SummaryTicket{{Address: "p:e/01"}}, "Paused. 1 ticket still running."},
		{[]wire.SummaryTicket{{Address: "p:e/01"}, {Address: "p:e/02"}, {Address: "p:e/03"}}, "Paused. 3 tickets still running."},
	}
	for _, tc := range cases {
		h.Summaries = map[string]wire.EpicSummary{"p:e": {Active: tc.active}}
		lines := h.BodyLines()
		if got := ansi.Strip(lines[0]); got != tc.want {
			t.Errorf("header = %q, want %q", got, tc.want)
		}
	}
}

// TestQueueHeaderCostSuffixFormats covers the bare-"$X"-vs-"$X of $Y" split on
// whether a soft limit is configured.
func TestQueueHeaderCostSuffixFormats(t *testing.T) {
	t.Parallel()
	h := testHeader(time.Now())
	h.Budget.Total = 42.3
	if got, want := h.CostSuffix(), "today $42.30"; got != want {
		t.Fatalf("budget disabled: CostSuffix() = %q, want bare %q", got, want)
	}

	h.Budget = wire.BudgetStatus{Total: 50, SoftLimit: 100}
	if got, want := h.CostSuffix(), "today $50.00 of $100.00"; got != want {
		t.Fatalf("soft limit set: CostSuffix() = %q, want %q", got, want)
	}
}

func statusRowAt(now time.Time, h Header, epic tickets.Epic) string {
	h.Now = func() time.Time { return now }
	return ansi.Strip(h.EpicStatusRow(epic, 0))
}

func TestEpicStatusRowElapsedAndAgentParts(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	h := testHeader(started)

	cases := []struct {
		name string
		now  time.Duration
		epic tickets.Epic
		want []string
		not  []string
	}{
		{"never started shows only the state", time.Hour,
			tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Status: "open"}}},
			[]string{"Waiting for a slot"}, []string{"elapsed", "agent", "·"}},
		{"started but idle uses the model clock, under a minute", 30 * time.Second,
			tickets.Epic{StartedAt: started, Tickets: []tickets.Ticket{{Identifier: "01", Status: "open"}}},
			[]string{"Waiting for a slot · <1m elapsed"}, []string{"agent"}},
		{"minute resolution", 134 * time.Minute,
			tickets.Epic{StartedAt: started, Tickets: []tickets.Ticket{{Identifier: "01", Status: "open"}}},
			[]string{"2h 14m elapsed"}, nil},
		{"agent time sums done tickets", 45 * time.Minute,
			tickets.Epic{StartedAt: started, Tickets: []tickets.Ticket{
				{Identifier: "01", Status: "done", ElapsedTime: 600}, {Identifier: "02", Status: "open"}}},
			[]string{"45m elapsed · 10m agent"}, []string{"· $"}},
		{"done uses start to completion", 10 * time.Hour,
			tickets.Epic{StartedAt: started, CompletedAt: started.Add(90 * time.Minute),
				Tickets: []tickets.Ticket{{Identifier: "01", Status: "done", ElapsedTime: 600}}},
			[]string{"1h 30m elapsed · 10m agent"}, nil},
		{"done without completed_at falls back to the ticket sum", 10 * time.Hour,
			tickets.Epic{StartedAt: started, Tickets: []tickets.Ticket{{Identifier: "01", Status: "done", ElapsedTime: 600}}},
			[]string{"10m elapsed · 10m agent"}, nil},
		{"done without started_at hides elapsed", 10 * time.Hour,
			tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Status: "done", ElapsedTime: 600}}},
			[]string{"10m agent"}, []string{" elapsed"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := statusRowAt(started.Add(tc.now), h, tc.epic)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("%q missing %q", got, w)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(got, n) {
					t.Errorf("%q should not contain %q", got, n)
				}
			}
			if strings.HasSuffix(strings.TrimSpace(got), "·") {
				t.Errorf("trailing separator: %q", got)
			}
		})
	}
}

func TestEpicStatusRowAgentTimeIncludesLiveRunningTicket(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	h := testHeader(started)
	h.RunningEpics = map[string]bool{"alpha": true}
	h.Live = map[string]map[string]rowfmt.LiveTicketState{"alpha": {"02": {Running: true, StartedAt: started.Add(30 * time.Minute)}}}
	epic := tickets.Epic{Name: "alpha", StartedAt: started, Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "done", ElapsedTime: 600}, {Identifier: "02", Status: "claimed"}}}

	got := statusRowAt(started.Add(45*time.Minute), h, epic)
	if !strings.Contains(got, "45m elapsed · 25m agent") {
		t.Errorf("want 10m done + 15m live = 25m agent: %q", got)
	}
}

func TestEpicStatusRowCutAtPaneEdge(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	h := testHeader(started.Add(time.Hour))
	epic := tickets.Epic{StartedAt: started, Tickets: []tickets.Ticket{{Identifier: "01", Status: "open"}}}

	if got := ansi.Strip(h.EpicStatusRow(epic, 20)); lipgloss.Width(got) > 20 {
		t.Errorf("width %d > 20: %q", lipgloss.Width(got), got)
	}
}
