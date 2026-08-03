package tickets

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

func newFlatModelForRowTests(epic tickets.Epic) FlatModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return FlatModel{
		loaded:   true,
		found:    true,
		epic:     epic,
		ordered:  epic.Tickets,
		live:     map[string]liveTicketState{},
		selected: -1,
		spinner:  sp,
	}
}

func TestRenderFlatTicketRow_NeverRun_SingleLine(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Title: "Open ticket", Status: "open"}}}
	m := newFlatModelForRowTests(epic)

	rows := m.renderFlatTicketRow(epic.Tickets[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 line for a never-run ticket, got %d: %#v", len(rows), rows)
	}
	if !strings.Contains(rows[0], "Open ticket") {
		t.Errorf("expected the title to render, got %q", rows[0])
	}
}

func TestRenderFlatTicketRow_Done_TwoLinesWithMetrics(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 754, ActualContextWindow: 45200},
	}}
	m := newFlatModelForRowTests(epic)

	rows := m.renderFlatTicketRow(epic.Tickets[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 lines for a done ticket, got %d: %#v", len(rows), rows)
	}
	if !strings.Contains(rows[0], "Done ticket") {
		t.Errorf("expected line 1 to keep the title, got %q", rows[0])
	}
	if !strings.Contains(rows[1], "12m34s") || !strings.Contains(rows[1], "45.2k tok") {
		t.Errorf("expected line 2 to show elapsed/tokens, got %q", rows[1])
	}
}

func TestRenderFlatTicketRow_Done_ZeroSentinelStillTwoLines(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Repaired ticket", Status: "done"},
	}}
	m := newFlatModelForRowTests(epic)

	rows := m.renderFlatTicketRow(epic.Tickets[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 lines even for the 0/0 sentinel, got %d: %#v", len(rows), rows)
	}
	if !strings.Contains(rows[1], "0s") || !strings.Contains(rows[1], "0 tok") {
		t.Errorf("expected line 2 to read 0s · 0 tok, got %q", rows[1])
	}
}

func TestRenderFlatTicketRow_Running_TwoLinesBareTitleThenSuffixAndMetrics(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Title: "Running ticket", Status: "open"}}}
	m := newFlatModelForRowTests(epic)
	m.live["01"] = liveTicketState{
		running: true, label: "iter-01",
		startedAt: time.Now().Add(-754 * time.Second), tokens: 45200,
	}

	rows := m.renderFlatTicketRow(epic.Tickets[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 lines for a running ticket, got %d: %#v", len(rows), rows)
	}
	if strings.Contains(rows[0], "iter-01") || strings.Contains(rows[0], "implementing") {
		t.Errorf("expected line 1 to be bare spinner+title, got %q", rows[0])
	}
	if !strings.Contains(rows[0], "Running ticket") {
		t.Errorf("expected line 1 to show the title, got %q", rows[0])
	}
	if !strings.Contains(rows[1], "iter-01") || !strings.Contains(rows[1], "(implementing...)") {
		t.Errorf("expected line 2 to carry the label/phase suffix, got %q", rows[1])
	}
	if !strings.Contains(rows[1], "12m34s") || !strings.Contains(rows[1], "45.2k tok") {
		t.Errorf("expected line 2 to show elapsed/tokens, got %q", rows[1])
	}
}

func TestRenderFlatTicketRow_Paused_TwoLinesWithReasonAndFrozenTokens(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{{Identifier: "01", Title: "Paused ticket", Status: "open"}}}
	m := newFlatModelForRowTests(epic)
	m.live["01"] = liveTicketState{
		paused: true, pauseKind: ralphloop.PauseRateLimit, reason: "context budget exceeded",
		startedAt: time.Now().Add(-3900 * time.Second), tokens: 1_200_000,
	}

	rows := m.renderFlatTicketRow(epic.Tickets[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 lines for a paused ticket, got %d: %#v", len(rows), rows)
	}
	if !strings.Contains(rows[1], "context budget exceeded") {
		t.Errorf("expected line 2 to carry the pause reason, got %q", rows[1])
	}
	if !strings.Contains(rows[1], "1h05m") || !strings.Contains(rows[1], "1.2M tok") {
		t.Errorf("expected line 2 to show elapsed/tokens, got %q", rows[1])
	}
}

func TestRenderFlatTicketRow_Done_LandedPresentRendersLanded(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 754, ActualContextWindow: 45200},
	}}
	m := newFlatModelForRowTests(epic)
	m.landedOK = true
	m.landed = map[string]bool{"01": true}

	rows := m.renderFlatTicketRow(epic.Tickets[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 lines, got %d: %#v", len(rows), rows)
	}
	if !strings.Contains(rows[1], "landed") || strings.Contains(rows[1], "not landed") {
		t.Errorf("expected line 2 to show plain 'landed', got %q", rows[1])
	}
}

func TestRenderFlatTicketRow_Done_LandedAbsentRendersNotLanded(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 754, ActualContextWindow: 45200},
	}}
	m := newFlatModelForRowTests(epic)
	m.landedOK = true
	m.landed = map[string]bool{}

	rows := m.renderFlatTicketRow(epic.Tickets[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 lines, got %d: %#v", len(rows), rows)
	}
	if !strings.Contains(rows[1], "not landed") {
		t.Errorf("expected line 2 to show 'not landed', got %q", rows[1])
	}
}

func TestRenderFlatTicketRow_Done_LandedUnavailableRendersLandedQuestionMark(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 754, ActualContextWindow: 45200},
	}}
	m := newFlatModelForRowTests(epic)
	m.landedOK = false
	m.landed = nil

	rows := m.renderFlatTicketRow(epic.Tickets[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 lines, got %d: %#v", len(rows), rows)
	}
	if !strings.Contains(rows[1], "landed?") {
		t.Errorf("expected line 2 to show 'landed?', got %q", rows[1])
	}
}

func TestListLines_FlattensTwoLineRowsAndHighlightsBoth(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Open ticket", Status: "open"},
		{Identifier: "02", Title: "Done ticket", Status: "done", ElapsedTime: 5, ActualContextWindow: 100},
	}}
	m := newFlatModelForRowTests(epic)
	m.selected = 1 // the done ticket, which renders two lines

	lines := m.listLines()
	if len(lines) != 3 {
		t.Fatalf("expected 1 line + 2 lines = 3 total, got %d: %#v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "Open ticket") {
		t.Errorf("expected lines[0] to be the open ticket's single line, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "Done ticket") || !strings.Contains(lines[2], "5s") {
		t.Errorf("expected lines[1]/[2] to be the done ticket's two lines, got %q / %q", lines[1], lines[2])
	}
}
