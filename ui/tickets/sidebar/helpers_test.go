package sidebar

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/tree"
)

// testEnv is the env the tab hands in for a loaded, focused sidebar.
func testEnv() Env { return sidebarEnvFor(rowTestModel{}) }

// scratchOf is where tickettest writes a fixture root's epics.
func scratchOf(root string) string { return filepath.Join(root, ".scratch") }

// reload feeds one disk load of root into s, as the tab does on every
// epicsLoadedMsg; firstLoad also puts the cursor on the first epic.
func reload(t *testing.T, s *Model, root string, firstLoad bool) {
	t.Helper()
	epics, err := tickets.Load(scratchOf(root))
	if err != nil {
		t.Fatal(err)
	}
	count, _ := tickets.CountArchivedEpics(scratchOf(root))
	s.Epics = epics
	s.ArchivedEpicCount = count
	s.ArchivedLazy.SetCount(count)
	s.ClampSelected(testEnv())
	if firstLoad {
		s.SelectFirstEpic()
	}
}

// loadedSidebar is a first load of root with every open epic expanded, so a
// test about rows inside an epic can see them without its own expand step.
func loadedSidebar(t *testing.T, root string) Model {
	t.Helper()
	s := loadedSidebarDefaults(t, root)
	for _, e := range s.Epics {
		if !e.AllDone() {
			s.ExplicitCollapsed[e.Path] = false
		}
	}
	s.ClampSelected(testEnv())
	return s
}

// loadedSidebarDefaults is a first load of root with the default collapse state.
func loadedSidebarDefaults(t *testing.T, root string) Model {
	t.Helper()
	s := New(scratchOf(root))
	s.Tree.SetVisibleHeight(40)
	reload(t, &s, root, true)
	return s
}

// view renders the sidebar body the way the tab's panel does at 120x40.
func view(s Model) string {
	return strings.Join(s.Body(testEnv(), 40, 120), "\n")
}

func findEpic(t *testing.T, s Model, name string) tickets.Epic {
	t.Helper()
	return s.Epics[indexOfEpic(t, s, name)]
}

func indexOfEpic(t *testing.T, s Model, name string) int {
	t.Helper()
	for i, e := range s.Epics {
		if e.Name == name {
			return i
		}
	}
	t.Fatalf("epic %q not found", name)
	return -1
}

// setCollapsedEpic mirrors a user's explicit toggle: it writes the durable
// ExplicitCollapsed map, then rebuilds so Entries() sees the change.
func setCollapsedEpic(s *Model, epicIdx int, collapsed bool) {
	s.ExplicitCollapsed[s.Epics[epicIdx].Path] = collapsed
	s.ClampSelected(testEnv())
}

// setCollapsedSection mirrors setCollapsedEpic for a section's own root row.
func setCollapsedSection(s *Model, section Section, collapsed bool) {
	s.ExplicitCollapsed[SectionID(section)] = collapsed
	s.ClampSelected(testEnv())
}

// visibleRows is every cursor-reachable row (epics + tickets), section
// headers excluded.
func visibleRows(s Model) []Row {
	var rows []Row
	for _, e := range s.Tree.Entries() {
		if r, ok := RowFromEntry(e); ok {
			rows = append(rows, r)
		}
	}
	return rows
}

// archivedHeaderIndex is the entry index of the Archived section's header.
func archivedHeaderIndex(t *testing.T, s Model) int {
	t.Helper()
	for i, e := range s.Tree.Entries() {
		if e.ID == SectionID(SectionArchived) {
			return i
		}
	}
	t.Fatal("archived section header not found")
	return -1
}

// toggleArchivedHeader is the expand/collapse key on the Archived header. It
// returns the load command an expand owes.
func toggleArchivedHeader(t *testing.T, s *Model) tea.Cmd {
	t.Helper()
	idx := archivedHeaderIndex(t, *s)
	if !s.Tree.ToggleEntry(idx) {
		t.Fatal("expected the Archived header to toggle")
	}
	return s.AfterToggle(testEnv(), SectionID(SectionArchived), nil)
}

// deliverCmd runs cmd (unwrapping tea.BatchMsg) and feeds each lazy-load
// result back into s, as the tab does.
func deliverCmd(s *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			deliverCmd(s, c)
		}
		return
	}
	if res, ok := msg.(tree.LazyResultMsg[tickets.Epic]); ok && s.ArchivedLazy.Deliver(res) {
		s.ArchivedEpics = s.ArchivedLazy.Rows()
		s.ClampSelected(testEnv())
	}
}
