package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/sidebar"
)

func TestSearch_SlashEntersInputMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(Model)

	if m.search.Mode() != search.SearchModeInput {
		t.Fatalf("expected search input mode after '/', got mode=%v", m.search.Mode())
	}
}

func TestSearch_MatchesTitleAndStatusWordWithoutHidingRows(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	beforeRows := len(visibleRows(m))

	m.search.Start("done")
	m.recomputeSearchMatches()

	if m.search.MatchesCount() != 1 {
		t.Fatalf("expected exactly one match for status word %q, got %d", "done", m.search.MatchesCount())
	}
	// Both tickets' rows must still be present — highlight-in-place, not filtering.
	if len(visibleRows(m)) != beforeRows {
		t.Fatalf("expected row count unchanged while searching, got %d want %d", len(visibleRows(m)), beforeRows)
	}

	content := m.View().Content
	if !strings.Contains(content, "First ticket") || !strings.Contains(content, "Second ticket") {
		t.Fatalf("expected both tickets still rendered during search, got:\n%s", content)
	}
}

func TestSearch_NonMatchesAreDimmed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.search.Start("first")
	m.recomputeSearchMatches()

	dimPrefix := strings.SplitN(ui.StyleDim.Render("PROBE"), "PROBE", 2)[0]

	epic := m.sidebar.Epics[0]
	// Entries: 0 = "Open epics" section header, 1 = the epic, 2/3 = its two
	// tickets — matching m.sidebar.tree.Entries()' actual indices.
	matchedLine := m.sidebar.RenderTicketRow(sidebarEnvFor(m), epic, sidebar.Row{TicketIdx: 0}, 2)[0] // "first ticket" row
	nonMatchedLine := m.sidebar.RenderTicketRow(sidebarEnvFor(m), epic, sidebar.Row{TicketIdx: 1}, 3)[0]

	if strings.Contains(matchedLine, dimPrefix) {
		t.Fatalf("expected matching row undimmed, got: %q", matchedLine)
	}
	if !strings.Contains(nonMatchedLine, dimPrefix) {
		t.Fatalf("expected non-matching row dimmed while searching, got: %q", nonMatchedLine)
	}
}

func TestSearch_EnterDismissesInputButKeepsMatches(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.search.Start("first")
	m.recomputeSearchMatches()
	if m.search.MatchesCount() == 0 {
		t.Fatalf("expected a match before dismissing")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if m.search.Mode() == search.SearchModeInput {
		t.Fatalf("expected enter to leave input mode")
	}
	if m.search.MatchesCount() == 0 {
		t.Fatalf("expected matches to persist after enter")
	}
}

func TestSearch_EscFullyClears(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.search.Start("first")
	m.recomputeSearchMatches()

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)

	if m.search.Mode() != search.SearchModeNone {
		t.Fatalf("expected esc to fully clear search mode, got %v", m.search.Mode())
	}
	if m.search.HasQuery() {
		t.Fatalf("expected esc to clear the query")
	}
}

func TestSearch_MatchesTicketInsideCollapsedClosedEpic(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "closed-epic", "01-hidden-gem.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "open-epic", "01-other.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	closedEpic := m.sidebar.Epics[0]
	if !closedEpic.AllDone() {
		t.Fatalf("expected closed-epic to be all-done in this fixture")
	}
	// The epic itself also defaults to collapsed (ticket 02), but that's
	// moot here since it's nested inside the collapsed Closed section root
	// and stays unreachable until the section expands regardless.
	if !m.sidebar.Tree.CollapsedIDs()[sidebar.SectionID(sidebar.SectionComplete)] {
		t.Fatalf("expected Closed section to start collapsed by default")
	}

	m.search.Start("hidden gem")
	m.recomputeSearchMatches()

	if m.search.MatchesCount() != 1 {
		t.Fatalf("expected one match for ticket inside collapsed epic, got %d", m.search.MatchesCount())
	}
	if m.sidebar.Tree.CollapsedIDs()[sidebar.SectionID(sidebar.SectionComplete)] {
		t.Fatalf("expected matching ticket's Closed section to auto-expand")
	}

	match, ok := m.search.Match(0)
	if !ok {
		t.Fatalf("expected a match at position 0")
	}
	entries := m.sidebar.Tree.Entries()
	if match.DataIndex < 0 || match.DataIndex >= len(entries) {
		t.Fatalf("match DataIndex %d out of range of %d sidebar entries", match.DataIndex, len(entries))
	}
	r, ok := sidebar.RowFromEntry(entries[match.DataIndex])
	if !ok || r.IsEpic() || m.sidebar.Epics[r.EpicIdx].Tickets[r.TicketIdx].Title != "Hidden gem" {
		t.Fatalf("expected match to point at the hidden-gem ticket row, got row %+v ok=%v", r, ok)
	}
}

// TestSearch_MatchExpandsClosedSectionButNotManuallyCollapsedEpic covers
// ticket 02's requirement that a user's explicit collapse beats the search
// override in either direction: the Closed section itself has no explicit
// entry here, so search's transient override still expands it, but the
// epic the user explicitly collapsed stays collapsed and its ticket stays
// out of the rendered (and thus matchable) sidebar entries.
func TestSearch_MatchExpandsClosedSectionButNotManuallyCollapsedEpic(t *testing.T) {
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "closed-epic", "01-hidden-gem.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "open-epic", "01-other.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	closedEpicIdx := -1
	for i, e := range m.sidebar.Epics {
		if e.AllDone() {
			closedEpicIdx = i
		}
	}
	if closedEpicIdx == -1 {
		t.Fatalf("expected a closed epic in this fixture")
	}
	// The epic already defaults to collapsed once the Closed section is
	// expanded (ticket 02); collapse it explicitly here so the assertions
	// below exercise the explicit-beats-search-override path, not just the
	// declared default.
	setCollapsedEpic(&m, closedEpicIdx, true)
	if !m.sidebar.Tree.CollapsedIDs()[sidebar.SectionID(sidebar.SectionComplete)] {
		t.Fatalf("expected Closed section to start collapsed by default")
	}
	if !m.sidebar.Tree.CollapsedIDs()[m.sidebar.Epics[closedEpicIdx].Path] {
		t.Fatalf("expected closed-epic to be individually collapsed")
	}

	m.search.Start("hidden gem")
	m.recomputeSearchMatches()

	if m.search.MatchesCount() != 0 {
		t.Fatalf("expected no visible matches for a ticket inside a manually-collapsed epic, got %d", m.search.MatchesCount())
	}
	if m.sidebar.Tree.CollapsedIDs()[sidebar.SectionID(sidebar.SectionComplete)] {
		t.Fatalf("expected Closed section to auto-expand")
	}
	if !m.sidebar.Tree.CollapsedIDs()[m.sidebar.Epics[closedEpicIdx].Path] {
		t.Fatalf("expected the manually-collapsed epic to stay collapsed despite the search match")
	}
}

func TestSearch_NoMatchLeavesCollapseStateUnchanged(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "closed-epic", "01-hidden-gem.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "open-epic", "01-other.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if !m.sidebar.Tree.CollapsedIDs()[sidebar.SectionID(sidebar.SectionComplete)] {
		t.Fatalf("expected Closed section to start collapsed by default")
	}

	m.search.Start("no-such-term")
	m.recomputeSearchMatches()

	if m.search.MatchesCount() != 0 {
		t.Fatalf("expected zero matches, got %d", m.search.MatchesCount())
	}
	if !m.sidebar.Tree.CollapsedIDs()[sidebar.SectionID(sidebar.SectionComplete)] {
		t.Fatalf("expected non-matching search to leave collapse state unchanged")
	}
}

// TestSearch_SidebarHighlightsMatchExactlyOnce guards against ui/tree's
// generic highlighting stacking on top of an inline search.Highlight call in
// renderTicketRow (ticket 18): search.Highlight is a plain byte-substring
// wrapper, not idempotent, so applying it twice nests broken ANSI escapes
// around the match instead of highlighting it once cleanly.
func TestSearch_SidebarHighlightsMatchExactlyOnce(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.search.Start("first ticket")
	m.recomputeSearchMatches()
	if m.search.MatchesCount() != 1 {
		t.Fatalf("expected exactly one match, got %d", m.search.MatchesCount())
	}

	lines := m.sidebar.Body(sidebarEnvFor(m), 20, 80)
	var matchLine string
	for _, l := range lines {
		if strings.Contains(ansi.Strip(l), "First ticket") {
			matchLine = l
			break
		}
	}
	if matchLine == "" {
		t.Fatalf("expected a sidebar line containing the matched title, got:\n%#v", lines)
	}

	wrapped := ui.StyleActiveSearchResult.Render("First ticket")
	if strings.Count(matchLine, wrapped) != 1 {
		t.Fatalf("expected the match highlighted exactly once, got:\n%q", matchLine)
	}
	if !strings.Contains(ansi.Strip(matchLine), "First ticket") {
		t.Fatalf("expected plain title still present after stripping ANSI, got:\n%s", ansi.Strip(matchLine))
	}
}

func TestSearch_NAndShiftNCycleMatches(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "my-epic", "01-open-a.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "my-epic", "02-open-b.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.search.Start("open")
	m.recomputeSearchMatches()
	if m.search.MatchesCount() < 2 {
		t.Fatalf("expected at least two matches, got %d", m.search.MatchesCount())
	}
	m.search.DismissAndKeepResults()
	m.search.SetCursor(0)
	if match, ok := m.search.Match(0); ok {
		m.sidebar.Tree.SetSelectedIndex(match.DataIndex)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if match, ok := m.search.Match(1); !ok || m.sidebar.Tree.SelectedIndex() != match.DataIndex {
		t.Fatalf("expected n to move selection to next match")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N', Mod: tea.ModShift})
	m = updated.(Model)
	if match, ok := m.search.Match(0); !ok || m.sidebar.Tree.SelectedIndex() != match.DataIndex {
		t.Fatalf("expected shift+n to move selection back to previous match")
	}
}

func TestSearch_MatchesEpicRowsInRowOrderWithTickets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "widget-queue", "01-widget-polish.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "widget-queue", "02-other.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "unrelated", "01-nothing.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.search.Start("Widget")
	m.recomputeSearchMatches()

	if m.search.MatchesCount() != 2 {
		t.Fatalf("expected the epic row and one ticket to match, got %d", m.search.MatchesCount())
	}
	entries := m.sidebar.Tree.Entries()
	first, _ := m.search.Match(0)
	second, _ := m.search.Match(1)
	if first.DataIndex >= second.DataIndex {
		t.Fatalf("expected matches in row order, got %d then %d", first.DataIndex, second.DataIndex)
	}
	if e := entries[first.DataIndex].Value; e.Kind != sidebar.NodeEpic || m.sidebar.Epics[e.EpicIdx].Name != "widget-queue" {
		t.Fatalf("expected first match to be the widget-queue epic row, got %+v", e)
	}
	if e := entries[second.DataIndex].Value; e.Kind != sidebar.NodeTicket || m.sidebar.Epics[e.EpicIdx].Tickets[e.TicketIdx].Title != "Widget polish" {
		t.Fatalf("expected second match to be the widget-polish ticket row, got %+v", e)
	}

	dimPrefix := strings.SplitN(ui.StyleDim.Render("PROBE"), "PROBE", 2)[0]
	if line := m.sidebar.RenderEpicRow(sidebarEnvFor(m), m.sidebar.Epics[entries[first.DataIndex].Value.EpicIdx]); strings.Contains(line, dimPrefix) {
		t.Fatalf("expected matched epic row undimmed, got: %q", line)
	}
}

func TestSearch_EpicMatchExpandsClosedSectionButNotTheEpic(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "closed-epic", "01-done.md", "Status: done\n\nBody.\n")
	tickettest.WriteTicket(t, root, "open-epic", "01-other.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.search.Start("closed-epic")
	m.recomputeSearchMatches()

	if m.search.MatchesCount() != 1 {
		t.Fatalf("expected the closed epic row to match, got %d", m.search.MatchesCount())
	}
	if m.sidebar.Tree.CollapsedIDs()[sidebar.SectionID(sidebar.SectionComplete)] {
		t.Fatalf("expected the Closed section to auto-expand for an epic match")
	}
	closed := m.sidebar.Epics[0]
	if closed.Name != "closed-epic" {
		t.Fatalf("expected closed-epic at index 0, got %q", closed.Name)
	}
	if !m.sidebar.Tree.CollapsedIDs()[closed.Path] {
		t.Fatalf("expected an epic-only match to leave the epic itself collapsed")
	}
}

func TestSearch_MatchesTicketIDAndEpicName(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "27-second.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "beta", "01-other.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)

	m.search.Start("27")
	m.recomputeSearchMatches()
	if m.search.MatchesCount() != 1 {
		t.Fatalf("expected ticket id %q to match one ticket, got %d", "27", m.search.MatchesCount())
	}

	// Only the alpha epic row: its tickets' titles don't say alpha.
	m.search.Start("alpha")
	m.recomputeSearchMatches()
	if m.search.MatchesCount() != 1 {
		t.Fatalf("expected epic name %q to match only its epic row, got %d", "alpha", m.search.MatchesCount())
	}
}
