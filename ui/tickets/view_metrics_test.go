package tickets

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func newModelForTicketRowTests(epic tickets.Epic) Model {
	m := NewModel("", ui.Settings{}, keys.New(nil))
	m.loaded = true
	m.epics = []tickets.Epic{epic}
	m.collapsedEpics = map[string]bool{}
	return m
}

func TestRenderTicketRow_NeverRunIsSingleLine(t *testing.T) {
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{{Identifier: "01", Title: "Open ticket", Status: "open"}}}
	m := newModelForTicketRowTests(epic)

	lines := m.renderTicketRow(epic, epic.Tickets[0], 1)
	if len(lines) != 1 {
		t.Fatalf("renderTicketRow() returned %d lines, want 1: %#v", len(lines), lines)
	}
}

func TestRenderTicketRow_DoneHasMetricsLine(t *testing.T) {
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 754, ActualContextWindow: 45_200},
	}}
	m := newModelForTicketRowTests(epic)

	lines := m.renderTicketRow(epic, epic.Tickets[0], 1)
	if len(lines) != 2 {
		t.Fatalf("renderTicketRow() returned %d lines, want 2: %#v", len(lines), lines)
	}
	if !strings.Contains(lines[1], "12m34s") || !strings.Contains(lines[1], "45.2k tok") {
		t.Fatalf("metrics line = %q, want elapsed time and tokens", lines[1])
	}
}

func TestRenderTicketRow_LiveHasSuffixAndMetricsLine(t *testing.T) {
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{{Identifier: "01", Title: "Running ticket", Status: "claimed"}}}
	m := newModelForTicketRowTests(epic)
	m.implementingEpics = map[string]bool{epic.Name: true}
	m.live[epic.Name] = map[string]liveTicketState{
		"01": {
			running:   true,
			label:     "iter-01",
			startedAt: time.Now().Add(-754 * time.Second),
			tokens:    45_200,
		},
	}

	lines := m.renderTicketRow(epic, epic.Tickets[0], 1)
	if len(lines) != 2 {
		t.Fatalf("renderTicketRow() returned %d lines, want 2: %#v", len(lines), lines)
	}
	if strings.Contains(lines[0], "iter-01") || strings.Contains(lines[0], "implementing") {
		t.Errorf("title line contains live suffix: %q", lines[0])
	}
	for _, want := range []string{"iter-01", "implementing", "12m34s", "45.2k tok"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("metrics line = %q, want %q", lines[1], want)
		}
	}
}

func TestRenderTicketRow_PausedHasReasonAndMetricsLine(t *testing.T) {
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{{Identifier: "01", Title: "Paused ticket", Status: "claimed"}}}
	m := newModelForTicketRowTests(epic)
	m.implementingEpics = map[string]bool{epic.Name: true}
	m.live[epic.Name] = map[string]liveTicketState{
		"01": {
			paused:    true,
			pauseKind: ralphloop.PauseRateLimit,
			reason:    "context budget exceeded",
			startedAt: time.Now().Add(-3900 * time.Second),
			tokens:    1_200_000,
		},
	}

	lines := m.renderTicketRow(epic, epic.Tickets[0], 1)
	if len(lines) != 2 {
		t.Fatalf("renderTicketRow() returned %d lines, want 2: %#v", len(lines), lines)
	}
	for _, want := range []string{"context budget exceeded", "1h05m", "1.2M tok"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("metrics line = %q, want %q", lines[1], want)
		}
	}
}

func TestSidebarLineForSelectedCountsMetricsLines(t *testing.T) {
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done"},
		{Identifier: "02", Title: "Open ticket", Status: "open"},
	}}
	m := newModelForTicketRowTests(epic)
	m.selected = 2 // epic row, two-line done row, then open row

	line, height, ok := m.sidebarLineForSelected()
	if !ok {
		t.Fatal("sidebarLineForSelected() ok = false")
	}
	if line != 4 || height != 1 {
		t.Fatalf("sidebarLineForSelected() = (%d, %d), want (4, 1)", line, height)
	}
}

func TestSidebarLinesHighlightsBothLinesOfSelectedTicket(t *testing.T) {
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 5, ActualContextWindow: 100},
		{Identifier: "02", Title: "Open ticket", Status: "open"},
	}}
	m := newModelForTicketRowTests(epic)
	m.selected = 1 // epic is row 0; done ticket is row 1

	lines := m.sidebarLines()
	want := m.renderTicketRow(epic, epic.Tickets[0], 1)
	if len(lines) < 4 {
		t.Fatalf("sidebarLines() returned too few lines: %#v", lines)
	}
	for i := range want {
		if lines[2+i] != ui.RenderRowHighlight(want[i]) {
			t.Errorf("selected ticket line %d was not highlighted", i+1)
		}
	}
}
