// Package tree provides a generic, collapsible outline-tree UI component. A
// node carries both its own value and an optional list of children at the
// same time — there is no leaf-vs-directory split, no sorting, and no
// single-child-chain collapsing; traversal order is whatever the caller's
// ChildrenFunc returns.
package tree

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/search"
	"maps"
	"strings"
)

// Entry is a flattened, depth-annotated tree row.
type Entry[T any] struct {
	ID          string
	ParentID    string
	Depth       int
	Value       T
	HasChildren bool
	Expanded    bool
	// DisplayName is set by BuildEntriesFromPaths; BuildEntriesFromValues leaves it empty.
	DisplayName string
	// Leaves is every leaf value nested under this entry, collected recursively.
	// Set by BuildEntriesFromPaths; BuildEntriesFromValues leaves it nil.
	Leaves []T
	// Body holds extra physical lines rendered below the entry's primary
	// line (e.g. wrapped reason text). Nil/empty (the default) means a
	// single-line entry, preserving existing behavior for every consumer
	// that never sets it. Body is the only source of truth ui/tree uses for
	// both scroll/paging math and rendering, so the two can't drift apart.
	Body []string
}

// lineCount is how many physical lines the entry occupies (its primary line
// plus every Body line).
func (e Entry[T]) lineCount() int {
	return 1 + len(e.Body)
}

// Model owns tree list state (selection/scroll/search), reusing the same
// generic ui/list, ui/search and ui/keys wiring ui/commit and ui/status use.
type Model[T any] struct {
	entries      []Entry[T]
	collapsed    map[string]bool
	list         list.Model
	visibleH     int
	isSelectable IsSelectableFunc[T]

	search search.Model
	keys   keys.Manager
}

// IsSelectableFunc reports whether a value's row may hold the cursor
// selection. Unset (nil, the default) means every entry is selectable —
// consumers with decorative/non-leaf rows (blank separators, section
// placeholders) opt in via SetIsSelectable and SkipUnselectable instead of
// hand-rolling their own skip logic.
type IsSelectableFunc[T any] func(T) bool

// SetIsSelectable registers fn as the tree's selectability policy for
// SkipUnselectable. Each consumer keeps its own policy (which node kinds
// count as decorative); ui/tree only owns the generic skip mechanism.
func (m *Model[T]) SetIsSelectable(fn IsSelectableFunc[T]) {
	m.isSelectable = fn
}

func (m Model[T]) isEntrySelectable(idx int) bool {
	if m.isSelectable == nil || idx < 0 || idx >= len(m.entries) {
		return true
	}
	return m.isSelectable(m.entries[idx].Value)
}

// SkipUnselectable nudges the current selection off a non-selectable entry
// (per SetIsSelectable) in dir's direction (+1 down, -1 up). If dir's
// direction runs off the end of the entries while still on a non-selectable
// row (e.g. paging up lands back on entry 0 with nowhere further up to go),
// it retries the opposite direction — a non-selectable row must never end up
// selected, even at a list boundary. A no-op if SetIsSelectable was never
// called.
func (m *Model[T]) SkipUnselectable(dir int) {
	if m.isSelectable == nil {
		return
	}
	idx := m.skipUnselectableDir(m.list.Selected(), dir)
	if !m.isEntrySelectable(idx) {
		idx = m.skipUnselectableDir(idx, -dir)
	}
	if idx != m.list.Selected() {
		m.SetSelectedIndex(idx)
	}
}

func (m Model[T]) skipUnselectableDir(idx, dir int) int {
	for !m.isEntrySelectable(idx) {
		next := idx + dir
		if next < 0 || next >= len(m.entries) {
			break
		}
		idx = next
	}
	return idx
}

type Result struct {
	Handled             bool
	SelectionChanged    bool
	RebuildRequested    bool
	OpenSelected        bool
	SearchQueryChanged  bool
	SearchCursorChanged bool
}

// NewModel constructs a tree Model. extra bindings are appended to the base
// tree bindings before constructing the internal keys.Manager, so a consumer
// (e.g. ui/status's stage/discard bindings) gets chord-matching and
// help-screen registration for free. Dispatch is not automatic: the consumer
// still needs its own interception logic (a switch on the matched binding ID)
// to act on a matched extra binding — see ui/status/filetree_keys.go for the
// reference shape.
func NewModel[T any](extra ...keys.Binding) Model[T] {
	bindings := make([]keys.Binding, 0, len(treeBindings)+len(extra))
	bindings = append(bindings, treeBindings...)
	bindings = append(bindings, extra...)
	return Model[T]{
		collapsed: map[string]bool{},
		search:    search.NewModel(),
		keys:      keys.New(bindings),
	}
}

func (m Model[T]) Init() tea.Cmd {
	return nil
}

func (m Model[T]) Entries() []Entry[T] {
	return m.entries
}

func (m *Model[T]) SetEntries(entries []Entry[T]) {
	m.entries = entries
	// Re-clamp selection to the new entry count.
	m.list.SetSelected(m.list.Selected(), len(m.entries))
}

func (m Model[T]) SelectedIndex() int {
	return m.list.Selected()
}

func (m *Model[T]) SetSelectedIndex(index int) {
	m.list.SetSelected(index, len(m.entries))
	m.list.EnsureSelectionVisibleLines(len(m.entries), m.entryLineHeight, m.visibleH)
}

// SelectAtBodyLine resolves bodyLine — a tree-body-relative line (0 = the
// first rendered body line at the current ScrollOffset), as reported by a
// consumer's mouse click after it has already subtracted its own panel
// origin/header height — to the entry occupying that physical line, using
// ticket 26's line-aware list.ItemAtLine so mixed single-/multi-line entries
// resolve correctly. A click on a multi-line entry's body line resolves to
// that entry, not its successor. No-op if bodyLine falls past the last
// rendered line (never clamps onto the last entry) or lands on a row
// SetIsSelectable excludes.
func (m *Model[T]) SelectAtBodyLine(bodyLine int) {
	idx := list.ItemAtLine(m.list.Offset(), len(m.entries), m.entryLineHeight, bodyLine)
	if idx < 0 || !m.isEntrySelectable(idx) {
		return
	}
	m.SetSelectedIndex(idx)
}

// entryLineHeight is m.entries' list.LineHeight: it reports how many
// physical lines entry i occupies (see Entry.Body), defaulting to 1 for an
// out-of-range index.
func (m Model[T]) entryLineHeight(i int) int {
	if i < 0 || i >= len(m.entries) {
		return 1
	}
	return m.entries[i].lineCount()
}

// ScrollOffset returns the current scroll offset of the list.
func (m Model[T]) ScrollOffset() int {
	return m.list.Offset()
}

// SetVisibleHeight stores the visible height used for navigation and scroll.
func (m *Model[T]) SetVisibleHeight(h int) {
	m.visibleH = h
}

// ScrollViewport scrolls the viewport by delta rows without moving the
// selection (e.g. a mouse wheel pans the list independently of the cursor).
func (m *Model[T]) ScrollViewport(delta int) {
	m.list.ScrollOffsetOnlyLines(delta, len(m.entries), m.entryLineHeight, m.visibleH)
}

// ScrollPage moves selection and viewport together by delta (vim-style
// ctrl+d/u): delta's sign is direction, its magnitude is the line budget to
// page by (callers pass ±list.DefaultScroll).
func (m *Model[T]) ScrollPage(delta int) {
	dir, lineBudget := 1, delta
	if delta < 0 {
		dir, lineBudget = -1, -delta
	}
	m.list.ScrollPageLines(dir, len(m.entries), m.entryLineHeight, lineBudget)
}

func (m Model[T]) selectedEntry() (Entry[T], bool) {
	sel := m.list.Selected()
	if sel < 0 || sel >= len(m.entries) {
		return Entry[T]{}, false
	}
	return m.entries[sel], true
}

func (m Model[T]) CollapsedIDs() map[string]bool {
	out := make(map[string]bool, len(m.collapsed))
	maps.Copy(out, m.collapsed)
	return out
}

func (m *Model[T]) SetCollapsedIDs(ids map[string]bool) {
	m.collapsed = make(map[string]bool, len(ids))
	maps.Copy(m.collapsed, ids)
}

func (m *Model[T]) Search() *search.Model {
	return &m.search
}

func (m *Model[T]) SetSearchMatches(matches []search.Match) {
	m.search.SetMatches(matches)
}

func (m *Model[T]) SetSearchMatchesAndJump(matches []search.Match) tea.Cmd {
	m.SetSearchMatches(matches)
	if match, ok := m.search.Match(m.search.Cursor()); ok {
		return searchJumpToMatchCmd(match)
	}

	return nil
}

func (m Model[T]) ComputeSearchMatches(query string, text func(Entry[T]) string) []search.Match {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []search.Match{}
	}

	var matches []search.Match
	for i, entry := range m.entries {
		if strings.Contains(strings.ToLower(text(entry)), q) {
			matches = append(matches, search.Match{DataIndex: i})
		}
	}
	return matches
}

func (m *Model[T]) RecomputeSearchMatches(text func(Entry[T]) string) {
	m.SetSearchMatches(m.ComputeSearchMatches(m.search.Query(), text))
}

func (m *Model[T]) ApplyPassiveSearch(query string, text func(Entry[T]) string) {
	m.search.SetPassiveResults(query, m.ComputeSearchMatches(query, text))
}

func (m *Model[T]) FocusCurrentSearchMatch() bool {
	match, ok := m.search.Match(m.search.Cursor())
	if !ok || match.DataIndex < 0 || match.DataIndex >= len(m.entries) {
		return false
	}
	prev := m.SelectedIndex()
	m.SetSelectedIndex(match.DataIndex)
	return m.SelectedIndex() != prev
}

func (m Model[T]) SearchMatch(index int) (matched bool, current bool) {
	if !m.search.HasQuery() {
		return false, false
	}
	pos, ok := m.search.MatchPosByDataIndex(index)
	return ok, ok && pos == m.search.Cursor()
}

func searchJumpToMatchCmd(match search.Match) tea.Cmd {
	return func() tea.Msg {
		return search.JumpToMatchMsg{Match: match}
	}
}

func (m *Model[T]) Keys() *keys.Manager {
	return &m.keys
}

func (m Model[T]) HasPendingChord() bool {
	return len(m.keys.Prefix()) > 0
}

func (m Model[T]) Update(msg tea.Msg) (Model[T], tea.Cmd, Result) {
	prevSelected := m.list.Selected()
	if nextSearch, cmd, result := m.search.Update(msg); result.Handled {
		m.search = nextSearch
		return m, cmd, Result{
			Handled:             true,
			SelectionChanged:    m.list.Selected() != prevSelected,
			SearchQueryChanged:  result.QueryChanged,
			SearchCursorChanged: result.CursorChanged,
		}
	}

	if key, ok := msg.(tea.KeyPressMsg); ok {
		match, consumed := m.keys.Process(key)
		if consumed && match == nil {
			return m, nil, Result{Handled: true} // chord in progress
		}
		if match != nil {
			switch match.ID {
			case BindingMoveDown:
				m.list.NavigateLines(+1, len(m.entries), m.entryLineHeight, m.visibleH)
				m.SkipUnselectable(+1)
				return m, nil, Result{Handled: true, SelectionChanged: m.list.Selected() != prevSelected}
			case BindingMoveUp:
				m.list.NavigateLines(-1, len(m.entries), m.entryLineHeight, m.visibleH)
				m.SkipUnselectable(-1)
				return m, nil, Result{Handled: true, SelectionChanged: m.list.Selected() != prevSelected}
			case BindingCollapse:
				if collapseSelected(m.entries, m.collapsed, m.list.Selected()) {
					return m, nil, Result{Handled: true, RebuildRequested: true}
				}
				if idx, ok := parentIndex(m.entries, m.list.Selected()); ok && idx != m.list.Selected() {
					m.list.SetSelected(idx, len(m.entries))
					return m, nil, Result{Handled: true, SelectionChanged: true}
				}
				return m, nil, Result{Handled: true}
			case BindingExpand:
				entry, ok := m.selectedEntry()
				if !ok {
					return m, nil, Result{Handled: true}
				}
				if !entry.HasChildren {
					return m, nil, Result{Handled: true, OpenSelected: true}
				}
				if expandSelected(m.entries, m.collapsed, m.list.Selected()) {
					return m, nil, Result{Handled: true, RebuildRequested: true}
				}
				if idx, ok := firstChildIndex(m.entries, m.list.Selected()); ok && idx != m.list.Selected() {
					m.list.SetSelected(idx, len(m.entries))
					return m, nil, Result{Handled: true, SelectionChanged: true}
				}
				return m, nil, Result{Handled: true}
			case BindingToggle:
				if toggleOnEnter(m.entries, m.collapsed, m.list.Selected()) {
					return m, nil, Result{Handled: true, RebuildRequested: true}
				}
				return m, nil, Result{Handled: true, OpenSelected: true}
			default:
				// parent-level binding — left to the embedding model to handle via Keys().Process
				return m, nil, Result{}
			}
		}
	}
	return m, nil, Result{}
}

func (m *Model[T]) ToggleSelected() bool {
	return toggleOnEnter(m.entries, m.collapsed, m.list.Selected())
}

func (m *Model[T]) CollapseSelected() bool {
	if collapseSelected(m.entries, m.collapsed, m.list.Selected()) {
		return true
	}
	if idx, ok := parentIndex(m.entries, m.list.Selected()); ok && idx != m.list.Selected() {
		m.list.SetSelected(idx, len(m.entries))
		return true
	}
	return false
}

func (m *Model[T]) ExpandSelected() bool {
	return expandSelected(m.entries, m.collapsed, m.list.Selected())
}

// MoveToAdjacentFile moves selection to the next (delta>0) or previous
// (delta<0) leaf row, skipping directory rows (entries with HasChildren).
// Returns false if there is no adjacent leaf in that direction.
func (m *Model[T]) MoveToAdjacentFile(delta int) bool {
	idx, ok := adjacentLeafIndex(m.entries, m.list.Selected(), delta)
	if !ok {
		return false
	}
	m.list.SetSelected(idx, len(m.entries))
	return true
}

func (m *Model[T]) FocusParent() bool {
	idx, ok := parentIndex(m.entries, m.list.Selected())
	if !ok || idx == m.list.Selected() {
		return false
	}
	m.list.SetSelected(idx, len(m.entries))
	return true
}
