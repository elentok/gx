package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/search"
)

func TestSearch_SlashEntersInputMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

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
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

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
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-second-ticket.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	m.search.Start("first")
	m.recomputeSearchMatches()

	dimPrefix := strings.SplitN(ui.StyleDim.Render("PROBE"), "PROBE", 2)[0]

	epic := m.epics[0]
	matchedLine := m.renderTicketRow(epic, row{ticketIdx: 0}, 1)[0] // "first ticket" row
	nonMatchedLine := m.renderTicketRow(epic, row{ticketIdx: 1}, 2)[0]

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
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

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
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")

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
	writeTicket(t, root, "closed-epic", "01-hidden-gem.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "open-epic", "01-other.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	closedEpic := m.epics[0]
	if !closedEpic.AllDone() {
		t.Fatalf("expected closed-epic to be all-done in this fixture")
	}
	if !m.isCollapsed(closedEpic) {
		t.Fatalf("expected closed epic to start collapsed by default")
	}

	m.search.Start("hidden gem")
	m.recomputeSearchMatches()

	if m.search.MatchesCount() != 1 {
		t.Fatalf("expected one match for ticket inside collapsed epic, got %d", m.search.MatchesCount())
	}
	if m.isCollapsed(m.epics[0]) {
		t.Fatalf("expected matching epic to auto-expand")
	}

	match, ok := m.search.Match(0)
	if !ok {
		t.Fatalf("expected a match at position 0")
	}
	entries := m.sidebarTree.Entries()
	if match.DataIndex < 0 || match.DataIndex >= len(entries) {
		t.Fatalf("match DataIndex %d out of range of %d sidebar entries", match.DataIndex, len(entries))
	}
	r, ok := rowFromEntry(entries[match.DataIndex])
	if !ok || r.isEpic() || m.epics[r.epicIdx].Tickets[r.ticketIdx].Title != "Hidden gem" {
		t.Fatalf("expected match to point at the hidden-gem ticket row, got row %+v ok=%v", r, ok)
	}
}

func TestSearch_NoMatchLeavesCollapseStateUnchanged(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "closed-epic", "01-hidden-gem.md", "Status: done\n\nBody.\n")
	writeTicket(t, root, "open-epic", "01-other.md", "Status: open\n\nBody.\n")

	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)

	if !m.isCollapsed(m.epics[0]) {
		t.Fatalf("expected closed epic to start collapsed by default")
	}

	m.search.Start("no-such-term")
	m.recomputeSearchMatches()

	if m.search.MatchesCount() != 0 {
		t.Fatalf("expected zero matches, got %d", m.search.MatchesCount())
	}
	if !m.isCollapsed(m.epics[0]) {
		t.Fatalf("expected non-matching search to leave collapse state unchanged")
	}
}

func TestSearch_NAndShiftNCycleMatches(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-open-a.md", "Status: open\n\nBody.\n")
	writeTicket(t, root, "my-epic", "02-open-b.md", "Status: open\n\nBody.\n")

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
		m.sidebarTree.SetSelectedIndex(match.DataIndex)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if match, ok := m.search.Match(1); !ok || m.sidebarTree.SelectedIndex() != match.DataIndex {
		t.Fatalf("expected n to move selection to next match")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N', Mod: tea.ModShift})
	m = updated.(Model)
	if match, ok := m.search.Match(0); !ok || m.sidebarTree.SelectedIndex() != match.DataIndex {
		t.Fatalf("expected shift+n to move selection back to previous match")
	}
}
