package tickets

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/queue"
	"github.com/elentok/gx/ui/tickets/rowfmt"
)

// TestQueueModelRowsRenderWithNoCheckbox covers ticket 08: the Queue tab is
// read-only for selection, so its rows must not render a checkbox glyph
// (checking/selecting only happens in the Tickets tab).
func TestQueueModelRowsRenderWithNoCheckbox(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	name := "01-first.md"
	tickettest.WriteTicket(t, root, "alpha", name, "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", name): true}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	content := m.View().Content
	for _, glyph := range []string{"[x]", "[ ]", "󰄲", "󰄱"} {
		if strings.Contains(content, glyph) {
			t.Fatalf("expected no checkbox glyph %q in Queue tab rows:\n%s", glyph, content)
		}
	}
}

// TestQueueModelShowsSameStatusAsTicketsTab covers ticket 25's first
// request: the Queue tab must show each ticket's status icon, blocked-by
// suffix, and (for a done ticket) the same elapsed/tokens metrics the
// Tickets tab renders (view.go's renderTicketRow), not just a bare checkbox
// and title.
func TestQueueModelShowsSameStatusAsTicketsTab(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-foundation.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-dependent.md", "Status: open\nBlocked by: 01\n\nBody.\n")
	tickettest.WriteRawQueueTicket(t, root, "alpha", "03-done.md", "---\nid: \"03\"\nstatus: done\ntype: implement\nactual_context_window: 12000\nelapsed_time: 754\n---\n\nDone.\n")

	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-foundation.md"): true,
		tickettest.TicketPath(root, "alpha", "02-dependent.md"):  true,
		tickettest.TicketPath(root, "alpha", "03-done.md"):       true,
	}
	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))
	content := m.View().Content

	if strings.Contains(content, "(blocked by 01)") {
		t.Fatalf("expected no blocked-by suffix without a server:\n%s", content)
	}
	if !strings.Contains(content, "12.0k tok") || !strings.Contains(content, "12m34s") {
		t.Fatalf("expected the done ticket's elapsed/tokens metrics line:\n%s", content)
	}
}

func TestRenderQueueTicketRow_CommitlessSuffix(t *testing.T) {
	t.Parallel()
	var m QueueModel
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Open ticket", Status: "open", Commitless: true},
	}}

	line := m.rows.RenderTicketRow(m.rowsEnv(), queue.Row{Epic: epic, Ticket: epic.Tickets[0]}, 0)
	if !strings.Contains(line, "Open ticket (commitless)") {
		t.Fatalf("title line = %q, want title followed by \" (commitless)\"", line)
	}
}

// TestRenderQueueTicketRow_IconColumnAlignsRegardlessOfChildren mirrors
// TestRenderTicketRow_IconColumnAlignsRegardlessOfChildren (ticket 10) for
// the Queue tab's own row renderer: same-depth siblings' icon columns must
// line up whether or not each one has children, since the triangle column is
// reserved at a fixed width for every row.
func TestRenderQueueTicketRow_IconColumnAlignsRegardlessOfChildren(t *testing.T) {
	t.Parallel()
	var m QueueModel
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Parent ticket", Status: "open"},
		{Identifier: "02", Title: "Leaf ticket", Status: "open"},
	}}

	withChildren := m.rows.RenderTicketRow(m.rowsEnv(), queue.Row{Epic: epic, Ticket: epic.Tickets[0], HasChildren: true, Expanded: true}, 0)
	childless := m.rows.RenderTicketRow(m.rowsEnv(), queue.Row{Epic: epic, Ticket: epic.Tickets[1]}, 1)

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

func leadingWhitespace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " "))]
}

// TestRenderQueueTicketRow_LiveRowIndentMatchesNormalRow covers ticket 10:
// rowfmt.RenderLiveTicketRow already prefixes its output with 2 spaces, so the
// caller must not add its own on top or a running row ends up indented 4
// spaces instead of 2.
func TestRenderQueueTicketRow_LiveRowIndentMatchesNormalRow(t *testing.T) {
	t.Parallel()
	var m QueueModel
	m.server.RunningEpics = map[string]bool{"epic": true}
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Normal ticket", Status: "open"},
		{Identifier: "02", Title: "Running ticket", Status: "claimed"},
	}}
	m.server.Live = map[string]map[string]rowfmt.LiveTicketState{
		"epic": {"02": {Running: true, Label: "iter-01"}},
	}

	normalLine := m.rows.RenderTicketRow(m.rowsEnv(), queue.Row{Epic: epic, Ticket: epic.Tickets[0]}, 0)
	liveLine := m.rows.RenderTicketRow(m.rowsEnv(), queue.Row{Epic: epic, Ticket: epic.Tickets[1]}, 1)

	normalIndent := leadingWhitespace(ansi.Strip(normalLine))
	liveIndent := leadingWhitespace(ansi.Strip(liveLine))
	if liveIndent != normalIndent {
		t.Fatalf("live row indent = %q, want %q (matching normal row): live=%q normal=%q", liveIndent, normalIndent, liveLine, normalLine)
	}
}

func TestRenderQueueTicketRow_DoneMetricsLineMatchesTitleColor(t *testing.T) {
	t.Parallel()
	var m QueueModel
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Done ticket", Status: "done", ElapsedTime: 5, ActualContextWindow: 100},
	}}

	line := m.rows.RenderTicketRow(m.rowsEnv(), queue.Row{Epic: epic, Ticket: epic.Tickets[0]}, 0)
	wantSuffix := " " + rowfmt.StatusDoneStyle.Italic(true).Render(rowfmt.FormatMetricsLine(5, 100, 0, false))
	if !strings.HasSuffix(line, wantSuffix) {
		t.Fatalf("row line = %q, want it to end with %q", line, wantSuffix)
	}
}

// TestRenderQueueTicketRow_NonDoneWithMetricsShowsLine covers ticket 03: a
// paused or claimed ticket that already carries
// non-zero elapsed_time/actual_context_window must show it, not just done
// tickets.
func TestRenderQueueTicketRow_NonDoneWithMetricsShowsLine(t *testing.T) {
	t.Parallel()
	var m QueueModel
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Paused ticket", Status: string(schema.StatusPaused), PauseReason: schema.PauseBlockedPane, ElapsedTime: 5, ActualContextWindow: 100},
	}}

	line := m.rows.RenderTicketRow(m.rowsEnv(), queue.Row{Epic: epic, Ticket: epic.Tickets[0]}, 0)
	if !strings.Contains(line, rowfmt.FormatMetricsLine(5, 100, 0, false)) {
		t.Fatalf("row line = %q, want it to contain metrics %q", line, rowfmt.FormatMetricsLine(5, 100, 0, false))
	}
}

// TestRenderQueueTicketRow_NonDoneWithZeroMetricsOmitsLine covers ticket 03:
// a non-done ticket with nothing recorded yet still omits the metrics line.
func TestRenderQueueTicketRow_NonDoneWithZeroMetricsOmitsLine(t *testing.T) {
	t.Parallel()
	var m QueueModel
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Paused ticket", Status: string(schema.StatusPaused), PauseReason: schema.PauseSpinning},
	}}

	line := m.rows.RenderTicketRow(m.rowsEnv(), queue.Row{Epic: epic, Ticket: epic.Tickets[0]}, 0)
	if strings.Contains(line, "tok") {
		t.Fatalf("row line = %q, want no metrics suffix", line)
	}
}

// TestRenderQueueTicketRow_SearchDimWinsOverNonCandidateDim covers ticket 13:
// a row that's both an injected non-candidate ancestor (ticket 08) and
// search-dimmed (an active query it doesn't match) must render its icon,
// title, badge, and suffix all in the same dim style — previously the title
// resolved nonCandidateDim first while the other elements resolved searchDim
// first, producing a two-tone row.
func TestRenderQueueTicketRow_SearchDimWinsOverNonCandidateDim(t *testing.T) {
	t.Parallel()
	var m QueueModel
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
		{Identifier: "01", Title: "Ancestor ticket", Status: "open"},
	}}
	m.search.Start("no-match")
	m.rows.Tree.Search().SetPassiveResults("no-match", nil)

	row := queue.Row{Epic: epic, Ticket: epic.Tickets[0], Actionable: false}
	line := m.rows.RenderTicketRow(m.rowsEnv(), row, 0)
	stripped := ansi.Strip(line)

	title := epic.Tickets[0].DisplayNumber() + " " + epic.Tickets[0].Title
	wantTitle := ui.StyleDim.Render(title)
	if !strings.Contains(line, wantTitle) {
		t.Fatalf("row line = %q, want title rendered in ui.StyleDim (matching icon/badge/suffix), got literal stripped text %q", line, stripped)
	}
	if strings.Contains(line, rowfmt.StatusDoneStyle.Render(title)) {
		t.Fatalf("row line = %q, title still rendered in statusDoneStyle instead of ui.StyleDim", line)
	}
}
