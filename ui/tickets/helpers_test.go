package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/tickets"
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
	for _, e := range m.sidebar.epics {
		if !e.AllDone() {
			m.sidebar.explicitCollapsed[e.Path] = false
		}
	}
	m.clampSelected()
}

// deliverLoadDefaults feeds one disk load of the fixture through Update. It
// skips Init, whose down fallback would also start a store watch.
func deliverLoadDefaults(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.Update(m.cmdLoadDisk()())
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

func findEpic(t *testing.T, m Model, name string) tickets.Epic {
	t.Helper()
	return m.sidebar.epics[indexOfEpic(t, m, name)]
}

func indexOfEpic(t *testing.T, m Model, name string) int {
	t.Helper()
	for i, e := range m.sidebar.epics {
		if e.Name == name {
			return i
		}
	}
	t.Fatalf("epic %q not found", name)
	return -1
}

// setCollapsedEpic mirrors a user's explicit toggle keypress: write directly
// into m.sidebar.explicitCollapsed (never m.sidebar.tree's own map, which
// clampSelected/refreshSidebarCollapse now overwrites from explicit state on
// every rebuild), then clampSelected to rebuild entries so a caller's
// subsequent Entries()/selectedRow() call sees the change.
func setCollapsedEpic(m *Model, epicIdx int, collapsed bool) {
	if m.sidebar.explicitCollapsed == nil {
		m.sidebar.explicitCollapsed = map[string]bool{}
	}
	m.sidebar.explicitCollapsed[m.sidebar.epics[epicIdx].Path] = collapsed
	m.clampSelected()
}

// setCollapsedSection mirrors setCollapsedEpic for a section's own root row
// (e.g. expanding the Closed section, which now starts collapsed by
// default, so a test can see its epics/tickets).
func setCollapsedSection(m *Model, section sidebarSection, collapsed bool) {
	if m.sidebar.explicitCollapsed == nil {
		m.sidebar.explicitCollapsed = map[string]bool{}
	}
	m.sidebar.explicitCollapsed[sidebarSectionID(section)] = collapsed
	m.clampSelected()
}

// selectedSidebarLine returns which physical line the currently selected
// entry renders on. Every sidebar row (including an empty-section
// placeholder) is now a real tree.Entry, so this is exactly the selected
// entry's index — no more header/placeholder line-counting.
func selectedSidebarLine(t *testing.T, m Model) int {
	t.Helper()
	return m.sidebar.tree.SelectedIndex()
}

// visibleRows mirrors the old m.visibleRows(): every cursor-reachable sidebar
// row (epics + tickets), section-header entries excluded.
func visibleRows(m Model) []row {
	var rows []row
	for _, e := range m.sidebar.tree.Entries() {
		if r, ok := rowFromEntry(e); ok {
			rows = append(rows, r)
		}
	}
	return rows
}

// archivedSectionHeaderIndex returns the sidebar entry index of the Archived
// section's own header row.
func archivedSectionHeaderIndex(t *testing.T, m Model) int {
	t.Helper()
	for i, e := range m.sidebar.tree.Entries() {
		if e.ID == sidebarSectionID(sectionArchived) {
			return i
		}
	}
	t.Fatal("archived section header not found")
	return -1
}
