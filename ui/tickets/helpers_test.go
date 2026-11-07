package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/tickets/sidebar"
)

// deliverLoad is deliverLoadDefaults plus expandOpenEpics.
func deliverLoad(t *testing.T, m Model) Model {
	t.Helper()
	m = deliverLoadDefaults(t, m)
	expandOpenEpics(&m)
	return m
}

// expandOpenEpics explicitly expands every open epic, so a test about rows
// inside an epic can see them without its own expand step.
func expandOpenEpics(m *Model) {
	for _, e := range m.sidebar.Epics {
		if !e.AllDone() {
			m.sidebar.ExplicitCollapsed[e.Path] = false
		}
	}
	m.clampSelected()
}

// deliverLoadDefaults feeds one disk load of the fixture through Update. It
// skips Init, whose down fallback would also start a store watch.
func deliverLoadDefaults(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.Update(cmdLoadDisk(m.scratchDir())())
	return updated.(Model)
}

// deliverCmd runs cmd (recursively unwrapping tea.BatchMsg) and feeds every
// resulting message back through Update, mirroring what the bubbletea
// runtime does for a batched command.
func deliverCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = deliverCmd(t, m, c)
		}
		return m
	}
	updated, _ := m.Update(msg)
	return updated.(Model)
}

// setCollapsedSection mirrors a user's explicit toggle of a section's root
// row: write the durable explicit map, then clampSelected to rebuild entries
// so a caller's subsequent Entries()/selectedRow() call sees the change.
func setCollapsedSection(m *Model, section sidebar.Section, collapsed bool) {
	if m.sidebar.ExplicitCollapsed == nil {
		m.sidebar.ExplicitCollapsed = map[string]bool{}
	}
	m.sidebar.ExplicitCollapsed[sidebar.SectionID(section)] = collapsed
	m.clampSelected()
}

// selectedSidebarLine returns which physical line the currently selected
// entry renders on. Every sidebar row (including an empty-section
// placeholder) is now a real tree.Entry, so this is exactly the selected
// entry's index — no more header/placeholder line-counting.
func selectedSidebarLine(t *testing.T, m Model) int {
	t.Helper()
	return m.sidebar.Tree.SelectedIndex()
}

// visibleRows mirrors the old m.visibleRows(): every cursor-reachable sidebar
// row (epics + tickets), section-header entries excluded.
func visibleRows(m Model) []sidebar.Row {
	var rows []sidebar.Row
	for _, e := range m.sidebar.Tree.Entries() {
		if r, ok := sidebar.RowFromEntry(e); ok {
			rows = append(rows, r)
		}
	}
	return rows
}

// archivedSectionHeaderIndex returns the sidebar entry index of the Archived
// section's own header row.
func archivedSectionHeaderIndex(t *testing.T, m Model) int {
	t.Helper()
	for i, e := range m.sidebar.Tree.Entries() {
		if e.ID == sidebar.SectionID(sidebar.SectionArchived) {
			return i
		}
	}
	t.Fatal("archived section header not found")
	return -1
}
