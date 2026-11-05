package sidebar

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"
)

func tk(id, title, status string) tickets.Ticket {
	n, _ := strconv.Atoi(id)
	return tickets.Ticket{Number: n, Identifier: id, Title: title, Status: status}
}

func ep(name string, ts ...tickets.Ticket) tickets.Epic {
	return tickets.Epic{Name: name, Path: name, Tickets: ts}
}

// newSearchSidebar builds a loaded sidebar over epics with every open epic
// explicitly expanded, like a tab that just finished its first load.
func newSearchSidebar(epics ...tickets.Epic) (Model, Env) {
	s := New("")
	env := Env{
		Loaded:              true,
		SessionMarker:       func(string) string { return "" },
		HasSuggestedActions: func(tickets.Epic, tickets.Ticket, bool) bool { return false },
		PendingText:         func(string) (string, bool) { return "", false },
	}
	s.Epics = epics
	for _, e := range epics {
		if !e.AllDone() {
			s.ExplicitCollapsed[e.Path] = false
		}
	}
	s.Tree.SetVisibleHeight(20)
	s.Rebuild(env)
	return s, env
}

func sectionCollapsed(s Model, section Section) bool {
	return s.Tree.CollapsedIDs()[SectionID(section)]
}

func TestSearch_SlashEntersInputMode(t *testing.T) {
	t.Parallel()
	s, env := newSearchSidebar(ep("my-epic", tk("01", "First ticket", "open")))

	s.UpdateSearch(env, tea.KeyPressMsg{Code: '/', Text: "/"})

	if s.Search.Mode() != search.SearchModeInput {
		t.Fatalf("expected search input mode after '/', got mode=%v", s.Search.Mode())
	}
}

func TestSearch_MatchesTitleAndStatusWordWithoutHidingRows(t *testing.T) {
	t.Parallel()
	s, env := newSearchSidebar(ep("my-epic", tk("01", "First ticket", "done"), tk("02", "Second ticket", "open")))
	beforeRows := len(s.Tree.Entries())

	s.Search.Start("done")
	s.RecomputeSearchMatches(env)

	if s.Search.MatchesCount() != 1 {
		t.Fatalf("expected exactly one match for status word %q, got %d", "done", s.Search.MatchesCount())
	}
	// Both tickets' rows must still be present — highlight-in-place, not filtering.
	if len(s.Tree.Entries()) != beforeRows {
		t.Fatalf("expected row count unchanged while searching, got %d want %d", len(s.Tree.Entries()), beforeRows)
	}

	content := strings.Join(s.Body(env, 20, 80), "\n")
	if !strings.Contains(content, "First ticket") || !strings.Contains(content, "Second ticket") {
		t.Fatalf("expected both tickets still rendered during search, got:\n%s", content)
	}
}

func TestSearch_NonMatchesAreDimmed(t *testing.T) {
	t.Parallel()
	s, env := newSearchSidebar(ep("my-epic", tk("01", "First ticket", "open"), tk("02", "Second ticket", "open")))

	s.Search.Start("first")
	s.RecomputeSearchMatches(env)

	dimPrefix := strings.SplitN(ui.StyleDim.Render("PROBE"), "PROBE", 2)[0]

	epic := s.Epics[0]
	// Entries: 0 = "Open epics" section header, 1 = the epic, 2/3 = its two
	// tickets — matching s.Tree.Entries()' actual indices.
	matchedLine := s.RenderTicketRow(env, epic, Row{TicketIdx: 0}, 2)[0] // "first ticket" row
	nonMatchedLine := s.RenderTicketRow(env, epic, Row{TicketIdx: 1}, 3)[0]

	if strings.Contains(matchedLine, dimPrefix) {
		t.Fatalf("expected matching row undimmed, got: %q", matchedLine)
	}
	if !strings.Contains(nonMatchedLine, dimPrefix) {
		t.Fatalf("expected non-matching row dimmed while searching, got: %q", nonMatchedLine)
	}
}

func TestSearch_EnterDismissesInputButKeepsMatches(t *testing.T) {
	t.Parallel()
	s, env := newSearchSidebar(ep("my-epic", tk("01", "First ticket", "open")))

	s.Search.Start("first")
	s.RecomputeSearchMatches(env)
	if s.Search.MatchesCount() == 0 {
		t.Fatalf("expected a match before dismissing")
	}

	s.UpdateSearch(env, tea.KeyPressMsg{Code: tea.KeyEnter})

	if s.Search.Mode() == search.SearchModeInput {
		t.Fatalf("expected enter to leave input mode")
	}
	if s.Search.MatchesCount() == 0 {
		t.Fatalf("expected matches to persist after enter")
	}
}

func TestSearch_EscFullyClears(t *testing.T) {
	t.Parallel()
	s, env := newSearchSidebar(ep("my-epic", tk("01", "First ticket", "open")))

	s.Search.Start("first")
	s.RecomputeSearchMatches(env)

	s.UpdateSearch(env, tea.KeyPressMsg{Code: tea.KeyEsc})

	if s.Search.Mode() != search.SearchModeNone {
		t.Fatalf("expected esc to fully clear search mode, got %v", s.Search.Mode())
	}
	if s.Search.HasQuery() {
		t.Fatalf("expected esc to clear the query")
	}
}

func TestSearch_MatchesTicketInsideCollapsedClosedEpic(t *testing.T) {
	t.Parallel()
	s, env := newSearchSidebar(
		ep("closed-epic", tk("01", "Hidden gem", "done")),
		ep("open-epic", tk("01", "Other", "open")),
	)

	if !s.Epics[0].AllDone() {
		t.Fatalf("expected closed-epic to be all-done in this fixture")
	}
	// The epic itself also defaults to collapsed (ticket 02), but that's
	// moot here since it's nested inside the collapsed Closed section root
	// and stays unreachable until the section expands regardless.
	if !sectionCollapsed(s, SectionComplete) {
		t.Fatalf("expected Closed section to start collapsed by default")
	}

	s.Search.Start("hidden gem")
	s.RecomputeSearchMatches(env)

	if s.Search.MatchesCount() != 1 {
		t.Fatalf("expected one match for ticket inside collapsed epic, got %d", s.Search.MatchesCount())
	}
	if sectionCollapsed(s, SectionComplete) {
		t.Fatalf("expected matching ticket's Closed section to auto-expand")
	}

	match, ok := s.Search.Match(0)
	if !ok {
		t.Fatalf("expected a match at position 0")
	}
	entries := s.Tree.Entries()
	if match.DataIndex < 0 || match.DataIndex >= len(entries) {
		t.Fatalf("match DataIndex %d out of range of %d sidebar entries", match.DataIndex, len(entries))
	}
	r, ok := RowFromEntry(entries[match.DataIndex])
	if !ok || r.IsEpic() || s.Epics[r.EpicIdx].Tickets[r.TicketIdx].Title != "Hidden gem" {
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
	t.Parallel()
	s, env := newSearchSidebar(
		ep("closed-epic", tk("01", "Hidden gem", "done")),
		ep("open-epic", tk("01", "Other", "open")),
	)

	// The epic already defaults to collapsed once the Closed section is
	// expanded (ticket 02); collapse it explicitly here so the assertions
	// below exercise the explicit-beats-search-override path, not just the
	// declared default.
	s.ExplicitCollapsed["closed-epic"] = true
	s.ClampSelected(env)
	if !sectionCollapsed(s, SectionComplete) {
		t.Fatalf("expected Closed section to start collapsed by default")
	}
	if !s.Tree.CollapsedIDs()["closed-epic"] {
		t.Fatalf("expected closed-epic to be individually collapsed")
	}

	s.Search.Start("hidden gem")
	s.RecomputeSearchMatches(env)

	if s.Search.MatchesCount() != 0 {
		t.Fatalf("expected no visible matches for a ticket inside a manually-collapsed epic, got %d", s.Search.MatchesCount())
	}
	if sectionCollapsed(s, SectionComplete) {
		t.Fatalf("expected Closed section to auto-expand")
	}
	if !s.Tree.CollapsedIDs()["closed-epic"] {
		t.Fatalf("expected the manually-collapsed epic to stay collapsed despite the search match")
	}
}

func TestSearch_NoMatchLeavesCollapseStateUnchanged(t *testing.T) {
	t.Parallel()
	s, env := newSearchSidebar(
		ep("closed-epic", tk("01", "Hidden gem", "done")),
		ep("open-epic", tk("01", "Other", "open")),
	)

	if !sectionCollapsed(s, SectionComplete) {
		t.Fatalf("expected Closed section to start collapsed by default")
	}

	s.Search.Start("no-such-term")
	s.RecomputeSearchMatches(env)

	if s.Search.MatchesCount() != 0 {
		t.Fatalf("expected zero matches, got %d", s.Search.MatchesCount())
	}
	if !sectionCollapsed(s, SectionComplete) {
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
	s, env := newSearchSidebar(ep("my-epic", tk("01", "First ticket", "open")))

	s.Search.Start("first ticket")
	s.RecomputeSearchMatches(env)
	if s.Search.MatchesCount() != 1 {
		t.Fatalf("expected exactly one match, got %d", s.Search.MatchesCount())
	}

	lines := s.Body(env, 20, 80)
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
	s, env := newSearchSidebar(ep("my-epic", tk("01", "Open a", "open"), tk("02", "Open b", "open")))

	s.Search.Start("open")
	s.RecomputeSearchMatches(env)
	if s.Search.MatchesCount() < 2 {
		t.Fatalf("expected at least two matches, got %d", s.Search.MatchesCount())
	}
	s.Search.DismissAndKeepResults()
	s.Search.SetCursor(0)
	if match, ok := s.Search.Match(0); ok {
		s.Tree.SetSelectedIndex(match.DataIndex)
	}

	s.UpdateSearch(env, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if match, ok := s.Search.Match(1); !ok || s.Tree.SelectedIndex() != match.DataIndex {
		t.Fatalf("expected n to move selection to next match")
	}

	s.UpdateSearch(env, tea.KeyPressMsg{Code: 'N', Text: "N", ShiftedCode: 'N', Mod: tea.ModShift})
	if match, ok := s.Search.Match(0); !ok || s.Tree.SelectedIndex() != match.DataIndex {
		t.Fatalf("expected shift+n to move selection back to previous match")
	}
}

func TestSearch_MatchesEpicRowsInRowOrderWithTickets(t *testing.T) {
	t.Parallel()
	s, env := newSearchSidebar(
		ep("unrelated", tk("01", "Nothing", "open")),
		ep("widget-queue", tk("01", "Widget polish", "open"), tk("02", "Other", "open")),
	)

	s.Search.Start("Widget")
	s.RecomputeSearchMatches(env)

	if s.Search.MatchesCount() != 2 {
		t.Fatalf("expected the epic row and one ticket to match, got %d", s.Search.MatchesCount())
	}
	entries := s.Tree.Entries()
	first, _ := s.Search.Match(0)
	second, _ := s.Search.Match(1)
	if first.DataIndex >= second.DataIndex {
		t.Fatalf("expected matches in row order, got %d then %d", first.DataIndex, second.DataIndex)
	}
	if e := entries[first.DataIndex].Value; e.Kind != NodeEpic || s.Epics[e.EpicIdx].Name != "widget-queue" {
		t.Fatalf("expected first match to be the widget-queue epic row, got %+v", e)
	}
	if e := entries[second.DataIndex].Value; e.Kind != NodeTicket || s.Epics[e.EpicIdx].Tickets[e.TicketIdx].Title != "Widget polish" {
		t.Fatalf("expected second match to be the widget-polish ticket row, got %+v", e)
	}

	dimPrefix := strings.SplitN(ui.StyleDim.Render("PROBE"), "PROBE", 2)[0]
	if line := s.RenderEpicRow(env, s.Epics[entries[first.DataIndex].Value.EpicIdx]); strings.Contains(line, dimPrefix) {
		t.Fatalf("expected matched epic row undimmed, got: %q", line)
	}
}

func TestSearch_EpicMatchExpandsClosedSectionButNotTheEpic(t *testing.T) {
	t.Parallel()
	s, env := newSearchSidebar(
		ep("closed-epic", tk("01", "Done", "done")),
		ep("open-epic", tk("01", "Other", "open")),
	)

	s.Search.Start("closed-epic")
	s.RecomputeSearchMatches(env)

	if s.Search.MatchesCount() != 1 {
		t.Fatalf("expected the closed epic row to match, got %d", s.Search.MatchesCount())
	}
	if sectionCollapsed(s, SectionComplete) {
		t.Fatalf("expected the Closed section to auto-expand for an epic match")
	}
	closed := s.Epics[0]
	if closed.Name != "closed-epic" {
		t.Fatalf("expected closed-epic at index 0, got %q", closed.Name)
	}
	if !s.Tree.CollapsedIDs()[closed.Path] {
		t.Fatalf("expected an epic-only match to leave the epic itself collapsed")
	}
}

func TestSearch_MatchesTicketIDAndEpicName(t *testing.T) {
	t.Parallel()
	s, env := newSearchSidebar(
		ep("alpha", tk("01", "First", "open"), tk("27", "Second", "open")),
		ep("beta", tk("01", "Other", "open")),
	)

	s.Search.Start("27")
	s.RecomputeSearchMatches(env)
	if s.Search.MatchesCount() != 1 {
		t.Fatalf("expected ticket id %q to match one ticket, got %d", "27", s.Search.MatchesCount())
	}

	// Only the alpha epic row: its tickets' titles don't say alpha.
	s.Search.Start("alpha")
	s.RecomputeSearchMatches(env)
	if s.Search.MatchesCount() != 1 {
		t.Fatalf("expected epic name %q to match only its epic row, got %d", "alpha", s.Search.MatchesCount())
	}
}
