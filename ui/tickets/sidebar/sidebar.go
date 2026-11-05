package sidebar

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tree"
)

// Model owns the Model's rows and tree state. It reads everything outside
// that state through sidebarEnv, so it never reaches back into Model.
type Model struct {
	Epics []tickets.Epic
	// ArchivedEpics holds the Archived section's tickets.Epic data (ticket
	// 04's lazy load populates it) — deliberately separate from epics so
	// epicsLoadedMsg's wholesale replacement of epics on every load/
	// auto-refresh tick never wipes it. archivedEpicCount is the cheap
	// up-front count (see cmdLoad) that arrives on every load regardless of
	// whether ArchivedEpics itself has been populated yet.
	ArchivedEpics     []tickets.Epic
	ArchivedEpicCount int
	// ArchivedLazy drives the Archived section's on-expand load (ticket 04):
	// Expand() returns the load command the first time the section is
	// expanded, Deliver feeds the result back in, and SetCount (called on
	// every epicsLoadedMsg) auto-invalidates the cache if the archive's count
	// changed since it was loaded.
	ArchivedLazy tree.LazySection[tickets.Epic]

	// Tree owns the selection/scroll/collapse state (ticket 02e1): a
	// Tree.Model[sidebarNode] built from buildSidebarEntries.
	Tree tree.Model[Node]
	// ExplicitCollapsed is the durable collapse state (ticket 02 collapse-state
	// layering): only ever written to by the user's own expand/collapse/toggle
	// keypresses (see afterToggle), never by a declared default or the
	// transient search-match override. Every rebuild re-derives tree's actual
	// collapse map fresh from this plus epics/search state
	// (deriveCollapsedSidebar, via refreshCollapse) — this map itself is never
	// overwritten by that derived result, so a default or override can never
	// calcify into a permanent entry.
	ExplicitCollapsed map[string]bool
	// HideDone is the "tc" chord's toggle (ticket 08): when set, done tickets
	// are excluded from buildSidebarEntries()/body() navigation and rendering.
	// Epic done/total header counts read epic.Tickets directly
	// (renderEpicRow), so they're unaffected by this filter.
	HideDone bool
	// Search is the sidebar's own "/" search; matches are indices into Tree's
	// entries (see RecomputeSearchMatches).
	Search search.Model
}

// Env is what the sidebar needs from the rest of the tab, handed in per
// call (see sidebarEnvFor).
type Env struct {
	Icons ui.IconSet
	// Active: the sidebar holds keyboard focus.
	Active      bool
	Loaded      bool
	OlderServer bool

	SessionMarker       func(address string) string
	HasSuggestedActions func(epic tickets.Epic, ticket tickets.Ticket, archived bool) bool
	// PendingText is a queued ticket's explain verdict text.
	PendingText func(ticketPath string) (string, bool)
}

func New(scratchDir string) Model {
	t := tree.NewModel[Node]()
	t.SetIsSelectable(func(n Node) bool {
		return n.Kind != NodeLoading && n.Kind != NodeLoadError
	})
	return Model{
		Tree:              t,
		ExplicitCollapsed: map[string]bool{},
		Search:            search.NewModel(),
		ArchivedLazy: tree.NewLazySection(0, func() ([]tickets.Epic, error) {
			return tickets.LoadArchived(scratchDir)
		}),
	}
}

// Rebuild re-derives the collapse map and rebuilds the tree's entries.
func (s *Model) Rebuild(env Env) {
	s.refreshCollapse(s.Search.Query())
	s.Tree.SetEntries(s.BuildSidebarEntries(env))
}

// ClampSelected rebuilds the tree's entries from the current
// epics/hideDone/collapse state, e.g. after a collapse hides the rows below
// the selection — SetEntries re-clamps selection to the new entry count.
// An empty-section placeholder is a real tree.Entry row but must never hold
// the cursor, so every rebuild (including the very first, off of
// tree.NewModel's zero-value selection at index 0) nudges off one if the
// rebuilt entries left the selection sitting on one.
func (s *Model) ClampSelected(env Env) {
	s.Rebuild(env)
	s.Tree.SkipUnselectable(1)
}

// SelectFirstEpic puts the cursor on the first epic row, so a freshly loaded
// tab opens on an epic rather than on the section header above it.
func (s *Model) SelectFirstEpic() {
	for i, e := range s.Tree.Entries() {
		if e.Value.Kind == NodeEpic {
			s.Tree.SetSelectedIndex(i)
			return
		}
	}
}

// refreshCollapse re-derives the tree's collapse map fresh from
// explicitCollapsed/epics/query (deriveCollapsedSidebar) — every rebuild path
// funnels through here so the sidebar's collapse-state read sites
// (buildSidebarEntries, the section-header glyph, isCollapsed/renderEpicRow)
// always read through the one map that was actually handed to the tree.
func (s *Model) refreshCollapse(query string) {
	s.Tree.SetCollapsedIDs(DeriveCollapsed(s.ExplicitCollapsed, s.Epics, s.ArchivedEpics, query))
}

// AfterToggle is the bookkeeping owed after the tree flipped the collapse
// state of entry toggledID, whether by key or by a click on its icon. It
// returns cmd, plus the Archived section's load when that section was just
// expanded.
func (s *Model) AfterToggle(env Env, toggledID string, cmd tea.Cmd) tea.Cmd {
	// Record only toggledID's own resulting value into the durable
	// explicit map — never the whole CollapsedIDs() map, which would
	// reintroduce the read-back-and-write-back loop (ticket 02) that
	// made every declared default/search override permanent.
	if toggledID != "" {
		if s.ExplicitCollapsed == nil {
			s.ExplicitCollapsed = map[string]bool{}
		}
		s.ExplicitCollapsed[toggledID] = s.Tree.CollapsedIDs()[toggledID]
	}
	if toggledID == SectionID(SectionArchived) && !s.Tree.CollapsedIDs()[toggledID] && s.ArchivedEpicCount > 0 {
		if expandCmd := s.ArchivedLazy.Expand(); expandCmd != nil {
			cmd = tea.Batch(cmd, expandCmd)
		}
	}
	s.ClampSelected(env)
	return cmd
}

// SelectFirstRow/selectLastRow implement "gg"/"G": jump the selection to the
// first/last row.
func (s *Model) SelectFirstRow() {
	if len(s.Tree.Entries()) == 0 {
		return
	}
	s.Tree.SetSelectedIndex(0)
	s.Tree.SkipUnselectable(1)
}

func (s *Model) SelectLastRow() {
	n := len(s.Tree.Entries())
	if n == 0 {
		return
	}
	s.Tree.SetSelectedIndex(n - 1)
	s.Tree.SkipUnselectable(-1)
}

// SelectedRow returns the row currently under the selection, if any. A
// nodeSection selection reports false (rowFromEntry) — section rows are
// cursor-reachable but have no row representation (not checkable, no
// epic/ticket to preview). A sidebar whose entries were never built (e.g. a
// test constructing it with epics set directly, never routing it through a
// WindowSizeMsg/epicsLoadedMsg) has an empty tree — that's not "nothing
// selected", it's "entries were never built", so it falls back to the first
// epic rather than reporting no selection at all.
func (s Model) SelectedRow() (Row, bool) {
	entries := s.Tree.Entries()
	if len(entries) == 0 {
		if len(s.Epics) == 0 {
			return Row{}, false
		}
		return Row{EpicIdx: 0, TicketIdx: -1}, true
	}
	idx := s.Tree.SelectedIndex()
	if idx < 0 || idx >= len(entries) {
		return Row{}, false
	}
	return RowFromEntry(entries[idx])
}

// SelectRowAt selects the epic/ticket row on bodyLine; ok is false (and the
// selection unchanged) for a section header, a placeholder or no row there.
func (s *Model) SelectRowAt(bodyLine int) bool {
	idx, ok := s.Tree.EntryAtBodyLine(bodyLine)
	if !ok {
		return false
	}
	if _, ok := RowFromEntry(s.Tree.Entries()[idx]); !ok {
		return false
	}
	s.Tree.SetSelectedIndex(idx)
	return true
}

// ClickAt handles a left click on bodyLine: a click on the row's tree icon
// toggles it (toggled, with the entry's ID), any other click selects the row.
func (s *Model) ClickAt(x, bodyLine int, icons ui.IconSet) (toggledID string, toggled bool) {
	idx, ok := s.Tree.EntryAtBodyLine(bodyLine)
	if !ok {
		return "", false
	}
	entry := s.Tree.Entries()[idx]
	if rowfmt.OnTreeIcon(x, entry, 0, icons) && s.Tree.ToggleEntry(idx) {
		return entry.ID, true
	}
	s.Tree.SetSelectedIndex(idx)
	return "", false
}

// ScrollWheel scrolls the viewport without moving the selection.
func (s *Model) ScrollWheel(msg tea.MouseWheelMsg) {
	if dir, ok := ui.WheelDirection(msg); ok {
		s.Tree.ScrollViewport(dir * ui.WheelScrollLines)
	}
}
