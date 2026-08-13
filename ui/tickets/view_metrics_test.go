package tickets

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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
	t.Parallel()
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{{Identifier: "01", Title: "Open ticket", Status: "open"}}}
	m := newModelForTicketRowTests(epic)

	lines := m.renderTicketRow(epic, row{ticketIdx: 0}, 1)
	if len(lines) != 1 {
		t.Fatalf("renderTicketRow() returned %d lines, want 1: %#v", len(lines), lines)
	}
}

func TestRenderTicketRow_DoneHasMetricsLine(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 754, ActualContextWindow: 45_200},
	}}
	m := newModelForTicketRowTests(epic)

	lines := m.renderTicketRow(epic, row{ticketIdx: 0}, 1)
	if len(lines) != 1 {
		t.Fatalf("renderTicketRow() returned %d lines, want 1: %#v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "12m34s") || !strings.Contains(lines[0], "45.2k tok") {
		t.Fatalf("row line = %q, want elapsed time and tokens", lines[0])
	}
}

func TestRenderTicketRow_LiveHasSuffixAndMetricsLine(t *testing.T) {
	t.Parallel()
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

	lines := m.renderTicketRow(epic, row{ticketIdx: 0}, 1)
	if len(lines) != 1 {
		t.Fatalf("renderTicketRow() returned %d lines, want 1: %#v", len(lines), lines)
	}
	for _, want := range []string{"iter-01", "implementing", "12m34s", "45.2k tok"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("row line = %q, want %q", lines[0], want)
		}
	}
}

// TestRenderTicketRow_LiveRowIndentNotDoubled covers ticket 10 (and ticket
// 02's checkbox-prefix mirroring): renderTicketRow must pass its own
// checkbox prefix into renderLiveTicketRow rather than adding one on top of
// a hardcoded prefix, so a live row's indent matches its non-live siblings'
// checkbox column exactly.
func TestRenderTicketRow_LiveRowIndentNotDoubled(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{{Identifier: "01", Title: "Running ticket", Status: "claimed"}}}
	m := newModelForTicketRowTests(epic)
	m.implementingEpics = map[string]bool{epic.Name: true}
	live := liveTicketState{running: true, label: "iter-01"}
	m.live[epic.Name] = map[string]liveTicketState{"01": live}

	lines := m.renderTicketRow(epic, row{ticketIdx: 0}, 0)

	wantPrefix := "    " + strings.Repeat(" ", triangleColumnWidth(m.icons())) + " " + m.checkboxGlyph(m.isChecked(epic.Tickets[0].Path)) + " "
	wantBase, _, ok := renderLiveTicketRow(m.icons(), m.implementSpinner, epic.Tickets[0], live, wantPrefix)
	if !ok {
		t.Fatalf("renderLiveTicketRow() ok = false, want true")
	}
	if !strings.HasPrefix(lines[0], wantBase) {
		t.Fatalf("live row line = %q, want it to start with renderLiveTicketRow's own output %q (no extra caller prefix)", lines[0], wantBase)
	}
}

// TestRenderTicketRow_LiveRowIndentMatchesNonLiveSibling covers ticket 02:
// the Tickets tab's checkbox column (4 spaces + checkbox + space) must be
// mirrored into the live-row path, so a running ticket's row indent matches
// its non-running siblings' instead of falling back to a bare 2-space prefix.
func TestRenderTicketRow_LiveRowIndentMatchesNonLiveSibling(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Normal ticket", Status: "open"},
		{Identifier: "02", Title: "Running ticket", Status: "claimed"},
	}}
	m := newModelForTicketRowTests(epic)
	m.implementingEpics = map[string]bool{epic.Name: true}
	m.live[epic.Name] = map[string]liveTicketState{"02": {running: true, label: "iter-01"}}

	normalLine := m.renderTicketRow(epic, row{ticketIdx: 0}, 0)[0]
	liveLine := m.renderTicketRow(epic, row{ticketIdx: 1}, 1)[0]

	normalIndent := leadingWhitespace(ansi.Strip(normalLine))
	liveIndent := leadingWhitespace(ansi.Strip(liveLine))
	if liveIndent != normalIndent {
		t.Fatalf("live row indent = %q, want %q (matching non-live sibling): live=%q normal=%q", liveIndent, normalIndent, liveLine, normalLine)
	}
}

func TestRenderTicketRow_PausedHasReasonAndMetricsLine(t *testing.T) {
	t.Parallel()
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

	lines := m.renderTicketRow(epic, row{ticketIdx: 0}, 1)
	if len(lines) != 1 {
		t.Fatalf("renderTicketRow() returned %d lines, want 1: %#v", len(lines), lines)
	}
	for _, want := range []string{"context budget exceeded", "1h05m", "1.2M tok"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("row line = %q, want %q", lines[0], want)
		}
	}
}

func TestSidebarLineForSelectedCountsMetricsLines(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done"},
		{Identifier: "02", Title: "Open ticket", Status: "open"},
	}}
	m := newModelForTicketRowTests(epic)
	m.selected = 2 // epic row, single-line done row, then open row

	line, height, ok := m.sidebarLineForSelected()
	if !ok {
		t.Fatal("sidebarLineForSelected() ok = false")
	}
	if line != 3 || height != 1 {
		t.Fatalf("sidebarLineForSelected() = (%d, %d), want (3, 1)", line, height)
	}
}

func TestStatusIconAndStyle_Colors(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)
	cases := []struct {
		status tickets.RenderedStatus
		want   lipgloss.Style
	}{
		{tickets.StatusOpen, statusOpenStyle},
		{tickets.StatusClaimed, statusClaimedStyle},
		{tickets.StatusNeedsRepair, statusNeedsRepairStyle},
		{tickets.StatusNeedsAnswer, statusNeedsAnswerStyle},
		{tickets.StatusDone, statusDoneStyle},
		{tickets.StatusBlocked, statusBlockedStyle},
		{tickets.StatusWaitingForChildren, statusWaitingForChildrenStyle},
	}
	for _, c := range cases {
		_, style := statusIconAndStyle(icons, c.status)
		if style.GetForeground() != c.want.GetForeground() {
			t.Errorf("status %v: foreground = %v, want %v", c.status, style.GetForeground(), c.want.GetForeground())
		}
	}

	if icon, _ := statusIconAndStyle(icons, tickets.StatusWaitingForChildren); icon != icons.TicketWaitingForChildren {
		t.Errorf("StatusWaitingForChildren icon = %q, want icons.TicketWaitingForChildren %q", icon, icons.TicketWaitingForChildren)
	}

	// Open renders with no foreground override — the terminal's own default.
	if _, style := statusIconAndStyle(icons, tickets.StatusOpen); style.GetForeground() != (lipgloss.NoColor{}) {
		t.Errorf("StatusOpen foreground = %v, want lipgloss.NoColor{} (no override)", style.GetForeground())
	}
	// Claimed is orange, needs-repair is red, matching each other's new colors.
	if _, style := statusIconAndStyle(icons, tickets.StatusClaimed); style.GetForeground() != ui.ColorOrange {
		t.Errorf("StatusClaimed foreground = %v, want ColorOrange", style.GetForeground())
	}
	if _, style := statusIconAndStyle(icons, tickets.StatusNeedsRepair); style.GetForeground() != ui.ColorRed {
		t.Errorf("StatusNeedsRepair foreground = %v, want ColorRed", style.GetForeground())
	}
}

func TestRenderTicketRow_CommitlessSuffix(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Open ticket", Status: "open", Commitless: true},
	}}
	m := newModelForTicketRowTests(epic)

	lines := m.renderTicketRow(epic, row{ticketIdx: 0}, 1)
	if !strings.Contains(lines[0], "Open ticket (commitless)") {
		t.Fatalf("title line = %q, want title followed by \" (commitless)\"", lines[0])
	}
}

func TestRenderTicketRow_DoneMetricsLineMatchesTitleColor(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 5, ActualContextWindow: 100},
	}}
	m := newModelForTicketRowTests(epic)

	lines := m.renderTicketRow(epic, row{ticketIdx: 0}, 1)
	wantSuffix := " " + statusDoneStyle.Italic(true).Render(formatMetricsLine(5, 100, 0))
	if !strings.HasSuffix(lines[0], wantSuffix) {
		t.Fatalf("row line = %q, want it to end with %q", lines[0], wantSuffix)
	}
}

func TestRenderEpicRow_ShowsDurationOnlyWhenBothTimestampsSet(t *testing.T) {
	t.Parallel()
	started := time.Now().Add(-(2*time.Hour + 15*time.Minute))
	completed := time.Now()

	both := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{{Identifier: "01", Status: "done"}}, StartedAt: started, CompletedAt: completed}
	m := newModelForTicketRowTests(both)
	if line := m.renderEpicRow(both); !strings.Contains(line, "took 2h 15m") {
		t.Fatalf("both timestamps set: line = %q, want it to contain %q", line, "took 2h 15m")
	}

	missing := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{{Identifier: "01", Status: "done"}}, StartedAt: started}
	m = newModelForTicketRowTests(missing)
	if line := m.renderEpicRow(missing); strings.Contains(line, "took") {
		t.Fatalf("missing completed_at: line = %q, want no duration text", line)
	}
}

// TestRenderTicketRow_IconColumnAlignsRegardlessOfChildren covers ticket 10:
// same-depth siblings' icon columns must line up whether or not each one has
// children, since the triangle column left of the checkbox is reserved at a
// fixed width for every row (a childless row just shows a blank there).
func TestRenderTicketRow_IconColumnAlignsRegardlessOfChildren(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Parent ticket", Status: "open"},
		{Identifier: "02", Title: "Leaf ticket", Status: "open"},
	}}
	m := newModelForTicketRowTests(epic)

	withChildren := m.renderTicketRow(epic, row{ticketIdx: 0, hasChildren: true, expanded: true}, 1)[0]
	childless := m.renderTicketRow(epic, row{ticketIdx: 1}, 2)[0]

	iconOffset := func(line string) int {
		stripped := ansi.Strip(line)
		return lipgloss.Width(stripped[:strings.Index(stripped, m.icons().TicketOpen)])
	}
	if got, want := iconOffset(withChildren), iconOffset(childless); got != want {
		t.Fatalf("withChildren ticket's icon column = %d, want %d (same as childless sibling)\nchildless: %q\nwithChildren: %q", got, want, childless, withChildren)
	}
	if strings.Contains(ansi.Strip(childless), m.icons().TriangleExpanded) || strings.Contains(ansi.Strip(childless), m.icons().TriangleCollapsed) {
		t.Fatalf("childless row unexpectedly contains a triangle glyph: %q", childless)
	}
}

func TestSidebarLinesHighlightsBothLinesOfSelectedTicket(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 5, ActualContextWindow: 100},
		{Identifier: "02", Title: "Open ticket", Status: "open"},
	}}
	m := newModelForTicketRowTests(epic)
	m.selected = 1 // epic is row 0; done ticket is row 1

	lines := m.sidebarLines()
	want := m.renderTicketRow(epic, row{ticketIdx: 0}, 1)
	if len(lines) < 4 {
		t.Fatalf("sidebarLines() returned too few lines: %#v", lines)
	}
	for i := range want {
		if lines[2+i] != ui.RenderRowHighlight(want[i]) {
			t.Errorf("selected ticket line %d was not highlighted", i+1)
		}
	}
}
