package header

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/tickets/rowfmt"
)

func TestEpicBarFillFollowsStateColour(t *testing.T) {
	t.Parallel()
	sum := wire.EpicSummary{Done: 1, Total: 2}
	if got, want := epicBar(sum, EpicStatusProblemStyle), epicBar(sum, lipgloss.NewStyle()); got == want {
		t.Fatalf("bar fill ignored the state colour: %q", got)
	}
	if !strings.Contains(epicBar(sum, EpicStatusProblemStyle), EpicStatusProblemStyle.Render(strings.Repeat("▰", epicBarCells/2))) {
		t.Fatalf("bar fill not rendered in the state colour: %q", epicBar(sum, EpicStatusProblemStyle))
	}
}

func TestEpicStatusLinePrefersCompletionTimestampsOverElapsedSum(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	completed := started.Add(3*time.Hour + 30*time.Minute)
	now := time.Now()

	withTimestamps := tickets.Epic{
		Tickets:     []tickets.Ticket{{Identifier: "01", Status: "done", ElapsedTime: 754}},
		StartedAt:   started,
		CompletedAt: completed,
	}
	if got := epicElapsedPart(withTimestamps, now); got != "3h 30m elapsed" {
		t.Fatalf("epic with completion timestamps: got %q, want %q", got, "3h 30m elapsed")
	}

	withoutCompletion := withTimestamps
	withoutCompletion.CompletedAt = time.Time{}
	if got := epicElapsedPart(withoutCompletion, now); got != "13m elapsed" {
		t.Fatalf("epic without completion timestamp: got %q, want %q", got, "13m elapsed")
	}
}

func TestEpicStatusPartsArePureInNowAndLive(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	epic := tickets.Epic{
		Name:      "e",
		StartedAt: now.Add(-2 * time.Hour),
		Tickets: []tickets.Ticket{
			{Identifier: "01", Status: "done", ElapsedTime: 600, Compactions: 1},
			{Identifier: "02", Status: "claimed"},
		},
	}
	live := map[string]rowfmt.LiveTicketState{"02": {Running: true, StartedAt: now.Add(-30 * time.Minute)}}

	got := strings.Join(epicStatusParts(epic, now, live), " | ")
	if want := "2h 0m elapsed | 40m agent | 1 compact"; got != want {
		t.Fatalf("parts: got %q, want %q", got, want)
	}
	if got := epicAgentPart(epic, now, nil); got != "10m agent" {
		t.Fatalf("agent part without live tickets: got %q", got)
	}
	if got := pluralize(2, "compact"); got != "2 compacts" {
		t.Fatalf("pluralize: got %q", got)
	}
}

func TestEpicContextMetricsSumsCompactions(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Status: "done", Compactions: 2},
		{Identifier: "02", Status: "done", Compactions: 1},
		{Identifier: "03", Status: "open"}, // never run: contributes 0 compactions
	}}

	if compacts := epicContextMetrics(epic); compacts != 3 {
		t.Fatalf("got compacts=%d, want 3", compacts)
	}
}
