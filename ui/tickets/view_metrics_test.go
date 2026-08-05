package tickets

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

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

// TestRenderTicketRow_LiveRowIndentNotDoubled covers ticket 10:
// renderLiveTicketRow already prefixes its output with 2 spaces, so
// renderTicketRow must not add its own on top or a running/paused row ends
// up indented 4 spaces instead of 2 (the Tickets tab's non-live rows carry
// an extra checkbox column, tracked separately by tickets 13/15, so the
// live row's baseline indent is compared against renderLiveTicketRow's own
// 2-space prefix rather than a non-live row directly).
func TestRenderTicketRow_LiveRowIndentNotDoubled(t *testing.T) {
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{{Identifier: "01", Title: "Running ticket", Status: "claimed"}}}
	m := newModelForTicketRowTests(epic)
	m.implementingEpics = map[string]bool{epic.Name: true}
	live := liveTicketState{running: true, label: "iter-01"}
	m.live[epic.Name] = map[string]liveTicketState{"01": live}

	lines := m.renderTicketRow(epic, epic.Tickets[0], 0)

	wantBase, _, ok := renderLiveTicketRow(m.icons(), m.implementSpinner, epic.Tickets[0], live)
	if !ok {
		t.Fatalf("renderLiveTicketRow() ok = false, want true")
	}
	if lines[0] != wantBase {
		t.Fatalf("live row title line = %q, want renderLiveTicketRow's own output %q (no extra caller prefix)", lines[0], wantBase)
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

func TestStatusIconAndStyle_Colors(t *testing.T) {
	icons := ui.Icons(false)
	cases := []struct {
		status tickets.RenderedStatus
		want   lipgloss.Style
	}{
		{tickets.StatusOpen, statusOpenStyle},
		{tickets.StatusClaimed, statusClaimedStyle},
		{tickets.StatusNeedsAttention, statusNeedsAttentionStyle},
		{tickets.StatusNeedsInfo, statusNeedsInfoStyle},
		{tickets.StatusDone, statusDoneStyle},
		{tickets.StatusBlocked, statusBlockedStyle},
	}
	for _, c := range cases {
		_, style := statusIconAndStyle(icons, c.status)
		if style.GetForeground() != c.want.GetForeground() {
			t.Errorf("status %v: foreground = %v, want %v", c.status, style.GetForeground(), c.want.GetForeground())
		}
	}

	// Open renders with no foreground override — the terminal's own default.
	if _, style := statusIconAndStyle(icons, tickets.StatusOpen); style.GetForeground() != (lipgloss.NoColor{}) {
		t.Errorf("StatusOpen foreground = %v, want lipgloss.NoColor{} (no override)", style.GetForeground())
	}
	// Claimed is orange, needs-attention is red, matching each other's new colors.
	if _, style := statusIconAndStyle(icons, tickets.StatusClaimed); style.GetForeground() != ui.ColorOrange {
		t.Errorf("StatusClaimed foreground = %v, want ColorOrange", style.GetForeground())
	}
	if _, style := statusIconAndStyle(icons, tickets.StatusNeedsAttention); style.GetForeground() != ui.ColorRed {
		t.Errorf("StatusNeedsAttention foreground = %v, want ColorRed", style.GetForeground())
	}
}

func TestRenderTicketRow_CommitlessSuffix(t *testing.T) {
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Open ticket", Status: "open", Commitless: true},
	}}
	m := newModelForTicketRowTests(epic)

	lines := m.renderTicketRow(epic, epic.Tickets[0], 1)
	if !strings.Contains(lines[0], "Open ticket (commitless)") {
		t.Fatalf("title line = %q, want title followed by \" (commitless)\"", lines[0])
	}
}

func TestRenderTicketRow_DoneMetricsLineMatchesTitleColor(t *testing.T) {
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 5, ActualContextWindow: 100},
	}}
	m := newModelForTicketRowTests(epic)

	lines := m.renderTicketRow(epic, epic.Tickets[0], 1)
	wantMetrics := renderRowMetricsLine(formatMetricsLine(5, 100), statusDoneStyle)
	if lines[1] != wantMetrics {
		t.Fatalf("metrics line = %q, want %q", lines[1], wantMetrics)
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
