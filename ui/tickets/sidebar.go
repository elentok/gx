package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tree"
)

// sidebar owns the sidebar's rows and tree state. It reads everything outside
// that state through sidebarEnv, so it never reaches back into Model.
type sidebar struct {
	epics []tickets.Epic
	// archivedEpics holds the Archived section's tickets.Epic data (ticket
	// 04's lazy load populates it) — deliberately separate from epics so
	// epicsLoadedMsg's wholesale replacement of epics on every load/
	// auto-refresh tick never wipes it. archivedEpicCount is the cheap
	// up-front count (see cmdLoad) that arrives on every load regardless of
	// whether archivedEpics itself has been populated yet.
	archivedEpics     []tickets.Epic
	archivedEpicCount int
	// archivedLazy drives the Archived section's on-expand load (ticket 04):
	// Expand() returns the load command the first time the section is
	// expanded, Deliver feeds the result back in, and SetCount (called on
	// every epicsLoadedMsg) auto-invalidates the cache if the archive's count
	// changed since it was loaded.
	archivedLazy tree.LazySection[tickets.Epic]

	// tree owns the selection/scroll/collapse state (ticket 02e1): a
	// tree.Model[sidebarNode] built from buildSidebarEntries.
	tree tree.Model[sidebarNode]
	// explicitCollapsed is the durable collapse state (ticket 02 collapse-state
	// layering): only ever written to by the user's own expand/collapse/toggle
	// keypresses (see afterToggle), never by a declared default or the
	// transient search-match override. Every rebuild re-derives tree's actual
	// collapse map fresh from this plus epics/search state
	// (deriveCollapsedSidebar, via refreshCollapse) — this map itself is never
	// overwritten by that derived result, so a default or override can never
	// calcify into a permanent entry.
	explicitCollapsed map[string]bool
	// hideDone is the "tc" chord's toggle (ticket 08): when set, done tickets
	// are excluded from buildSidebarEntries()/body() navigation and rendering.
	// Epic done/total header counts read epic.Tickets directly
	// (renderEpicRow), so they're unaffected by this filter.
	hideDone bool
}

// sidebarEnv is what the sidebar needs from the rest of the tab, handed in per
// call (see sidebarEnvFor).
type sidebarEnv struct {
	query    string // the sidebar search's raw query
	hasQuery bool
	icons    ui.IconSet
	// active: the sidebar holds keyboard focus.
	active      bool
	loaded      bool
	olderServer bool

	sessionMarker       func(address string) string
	hasSuggestedActions func(epic tickets.Epic, ticket tickets.Ticket, archived bool) bool
	// pendingText is a queued ticket's explain verdict text.
	pendingText func(ticketPath string) (string, bool)
}

func newSidebar(scratchDir string) sidebar {
	t := tree.NewModel[sidebarNode]()
	t.SetIsSelectable(func(n sidebarNode) bool {
		return n.kind != nodeLoading && n.kind != nodeLoadError
	})
	return sidebar{
		tree:              t,
		explicitCollapsed: map[string]bool{},
		archivedLazy: tree.NewLazySection(0, func() ([]tickets.Epic, error) {
			return tickets.LoadArchived(scratchDir)
		}),
	}
}

// rebuild re-derives the collapse map and rebuilds the tree's entries.
func (s *sidebar) rebuild(env sidebarEnv) {
	s.refreshCollapse(env.query)
	s.tree.SetEntries(s.buildSidebarEntries(env))
}

// clampSelected rebuilds the tree's entries from the current
// epics/hideDone/collapse state, e.g. after a collapse hides the rows below
// the selection — SetEntries re-clamps selection to the new entry count.
// An empty-section placeholder is a real tree.Entry row but must never hold
// the cursor, so every rebuild (including the very first, off of
// tree.NewModel's zero-value selection at index 0) nudges off one if the
// rebuilt entries left the selection sitting on one.
func (s *sidebar) clampSelected(env sidebarEnv) {
	s.rebuild(env)
	s.tree.SkipUnselectable(1)
}

// selectFirstEpic puts the cursor on the first epic row, so a freshly loaded
// tab opens on an epic rather than on the section header above it.
func (s *sidebar) selectFirstEpic() {
	for i, e := range s.tree.Entries() {
		if e.Value.kind == nodeEpic {
			s.tree.SetSelectedIndex(i)
			return
		}
	}
}

// refreshCollapse re-derives the tree's collapse map fresh from
// explicitCollapsed/epics/query (deriveCollapsedSidebar) — every rebuild path
// funnels through here so the sidebar's collapse-state read sites
// (buildSidebarEntries, the section-header glyph, isCollapsed/renderEpicRow)
// always read through the one map that was actually handed to the tree.
func (s *sidebar) refreshCollapse(query string) {
	s.tree.SetCollapsedIDs(deriveCollapsedSidebar(s.explicitCollapsed, s.epics, s.archivedEpics, query))
}

// afterToggle is the bookkeeping owed after the tree flipped the collapse
// state of entry toggledID, whether by key or by a click on its icon. It
// returns cmd, plus the Archived section's load when that section was just
// expanded.
func (s *sidebar) afterToggle(env sidebarEnv, toggledID string, cmd tea.Cmd) tea.Cmd {
	// Record only toggledID's own resulting value into the durable
	// explicit map — never the whole CollapsedIDs() map, which would
	// reintroduce the read-back-and-write-back loop (ticket 02) that
	// made every declared default/search override permanent.
	if toggledID != "" {
		if s.explicitCollapsed == nil {
			s.explicitCollapsed = map[string]bool{}
		}
		s.explicitCollapsed[toggledID] = s.tree.CollapsedIDs()[toggledID]
	}
	if toggledID == sidebarSectionID(sectionArchived) && !s.tree.CollapsedIDs()[toggledID] && s.archivedEpicCount > 0 {
		if expandCmd := s.archivedLazy.Expand(); expandCmd != nil {
			cmd = tea.Batch(cmd, expandCmd)
		}
	}
	s.clampSelected(env)
	return cmd
}

// selectFirstRow/selectLastRow implement "gg"/"G": jump the selection to the
// first/last row.
func (s *sidebar) selectFirstRow() {
	if len(s.tree.Entries()) == 0 {
		return
	}
	s.tree.SetSelectedIndex(0)
	s.tree.SkipUnselectable(1)
}

func (s *sidebar) selectLastRow() {
	n := len(s.tree.Entries())
	if n == 0 {
		return
	}
	s.tree.SetSelectedIndex(n - 1)
	s.tree.SkipUnselectable(-1)
}

// selectedRow returns the row currently under the selection, if any. A
// nodeSection selection reports false (rowFromEntry) — section rows are
// cursor-reachable but have no row representation (not checkable, no
// epic/ticket to preview). A sidebar whose entries were never built (e.g. a
// test constructing it with epics set directly, never routing it through a
// WindowSizeMsg/epicsLoadedMsg) has an empty tree — that's not "nothing
// selected", it's "entries were never built", so it falls back to the first
// epic rather than reporting no selection at all.
func (s sidebar) selectedRow() (row, bool) {
	entries := s.tree.Entries()
	if len(entries) == 0 {
		if len(s.epics) == 0 {
			return row{}, false
		}
		return row{epicIdx: 0, ticketIdx: -1}, true
	}
	idx := s.tree.SelectedIndex()
	if idx < 0 || idx >= len(entries) {
		return row{}, false
	}
	return rowFromEntry(entries[idx])
}

// selectRowAt selects the epic/ticket row on bodyLine; ok is false (and the
// selection unchanged) for a section header, a placeholder or no row there.
func (s *sidebar) selectRowAt(bodyLine int) bool {
	idx, ok := s.tree.EntryAtBodyLine(bodyLine)
	if !ok {
		return false
	}
	if _, ok := rowFromEntry(s.tree.Entries()[idx]); !ok {
		return false
	}
	s.tree.SetSelectedIndex(idx)
	return true
}

// clickAt handles a left click on bodyLine: a click on the row's tree icon
// toggles it (toggled, with the entry's ID), any other click selects the row.
func (s *sidebar) clickAt(x, bodyLine int, icons ui.IconSet) (toggledID string, toggled bool) {
	idx, ok := s.tree.EntryAtBodyLine(bodyLine)
	if !ok {
		return "", false
	}
	entry := s.tree.Entries()[idx]
	if rowfmt.OnTreeIcon(x, entry, 0, icons) && s.tree.ToggleEntry(idx) {
		return entry.ID, true
	}
	s.tree.SetSelectedIndex(idx)
	return "", false
}

// scrollWheel scrolls the viewport without moving the selection.
func (s *sidebar) scrollWheel(msg tea.MouseWheelMsg) {
	if dir, ok := ui.WheelDirection(msg); ok {
		s.tree.ScrollViewport(dir * ui.WheelScrollLines)
	}
}
