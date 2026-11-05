package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

func TestQueueSearch_SlashEntersInputMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(QueueModel)

	if m.search.Mode() != search.SearchModeInput {
		t.Fatalf("expected search input mode after '/', got mode=%v", m.search.Mode())
	}
}

func TestQueueSearch_TypedCharactersFilterAndHighlight(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "02-second.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-first.md"):  true,
		tickettest.TicketPath(root, "alpha", "02-second.md"): true,
	}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	for _, r := range "/first" {
		updated, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(QueueModel)
	}

	if m.search.MatchesCount() != 1 {
		t.Fatalf("expected exactly one match for %q, got %d", "first", m.search.MatchesCount())
	}

	dimPrefix := strings.SplitN(ui.StyleDim.Render("PROBE"), "PROBE", 2)[0]
	var ticketRows []int
	var ticketValues []queueNode
	for i, e := range m.rows.queueTree.Entries() {
		if e.Value.kind == nodeQueueTicket {
			ticketRows = append(ticketRows, i)
			ticketValues = append(ticketValues, e.Value)
		}
	}
	if len(ticketRows) != 2 {
		t.Fatalf("expected 2 ticket rows, got %d: %v", len(ticketRows), ticketRows)
	}
	matchedLine := m.rows.renderQueueTicketRow(m.rowsEnv(), ticketValues[0].ticket, ticketRows[0])
	nonMatchedLine := m.rows.renderQueueTicketRow(m.rowsEnv(), ticketValues[1].ticket, ticketRows[1])
	if strings.Contains(matchedLine, dimPrefix) {
		t.Fatalf("expected matching row undimmed, got: %q", matchedLine)
	}
	if !strings.Contains(nonMatchedLine, dimPrefix) {
		t.Fatalf("expected non-matching row dimmed while searching, got: %q", nonMatchedLine)
	}
}

func TestQueueSearch_MatchesEpicHeaderRows(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-alpha-tweak.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "beta", "01-other.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-alpha-tweak.md"): true,
		tickettest.TicketPath(root, "beta", "01-other.md"):        true,
	}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	for _, r := range "/alpha" {
		updated, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(QueueModel)
	}

	if m.search.MatchesCount() != 2 {
		t.Fatalf("expected the alpha epic header and its ticket to match, got %d", m.search.MatchesCount())
	}
	entries := m.rows.queueTree.Entries()
	first, _ := m.search.Match(0)
	second, _ := m.search.Match(1)
	if e := entries[first.DataIndex].Value; e.kind != nodeEpicName || e.epic.Name != "alpha" {
		t.Fatalf("expected first match to be the alpha epic header, got %+v", e)
	}
	if e := entries[second.DataIndex].Value; e.kind != nodeQueueTicket || e.ticket.ticket.Title != "Alpha tweak" {
		t.Fatalf("expected second match to be the alpha-tweak ticket, got %+v", e)
	}
}

func TestQueueSearch_EscExitsSearchMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(QueueModel)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(QueueModel)

	if m.search.Mode() != search.SearchModeNone {
		t.Fatalf("expected esc to fully clear search mode, got %v", m.search.Mode())
	}
	if m.search.HasQuery() {
		t.Fatalf("expected esc to clear the query")
	}
}

func TestQueueSearch_EnterExitsInputButKeepsResults(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	for _, r := range "/first" {
		updated, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(QueueModel)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(QueueModel)

	if m.search.Mode() == search.SearchModeInput {
		t.Fatalf("expected enter to leave input mode")
	}
	if m.search.MatchesCount() == 0 {
		t.Fatalf("expected matches to persist after enter")
	}
}

// TestQueueSearch_DigitsTypeIntoQueryNotBoundKeys covers this Queue tab's
// digit-routing counterpart to the Tickets tab fix (ticket 14): while search
// input is active, digit keys must type into the query rather than falling
// through to any of handleQueueKey's own bindings.
func TestQueueSearch_DigitsTypeIntoQueryNotBoundKeys(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{tickettest.TicketPath(root, "alpha", "01-first.md"): true}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(QueueModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m = updated.(QueueModel)

	if m.search.Query() != "1" {
		t.Fatalf("expected digit to type into the search query, got query=%q", m.search.Query())
	}
	if !m.InputFocused() {
		t.Fatalf("expected InputFocused()=true while search input is active")
	}
}

func TestQueueSearch_MatchesTicketIDAndEpicName(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tickettest.WriteTicket(t, root, "alpha", "01-first.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "alpha", "27-second.md", "Status: open\n\nBody.\n")
	tickettest.WriteTicket(t, root, "beta", "01-other.md", "Status: open\n\nBody.\n")
	checked := map[string]bool{
		tickettest.TicketPath(root, "alpha", "01-first.md"):  true,
		tickettest.TicketPath(root, "alpha", "27-second.md"): true,
		tickettest.TicketPath(root, "beta", "01-other.md"):   true,
	}

	m := loadQueueModel(t, withChecked(NewQueueModel(root, ui.Settings{}, keys.Manager{}), checked))

	m.search.Start("27")
	m.recomputeQueueSearchMatches()
	if m.search.MatchesCount() != 1 {
		t.Fatalf("expected ticket id %q to match one ticket, got %d", "27", m.search.MatchesCount())
	}

	// Only the alpha epic header: its tickets' titles don't say alpha.
	m.search.Start("alpha")
	m.recomputeQueueSearchMatches()
	if m.search.MatchesCount() != 1 {
		t.Fatalf("expected epic name %q to match only its epic header, got %d", "alpha", m.search.MatchesCount())
	}
}
