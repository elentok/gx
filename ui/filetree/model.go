package filetree

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/search"
	"maps"
	"strings"
)

// EntryKind identifies whether a row represents a directory or file.
type EntryKind int

const (
	EntryDir EntryKind = iota
	EntryFile
)

// Entry is a filetree row owned by the filetree child model.
type Entry[T any] struct {
	Kind        EntryKind
	Path        string
	ParentPath  string
	Depth       int
	DisplayName string
	Expanded    bool
	Value       T
	Leaves      []T
}

// Model owns the status/filetree list state and its local search state.
//
// Note: this is intentionally introduced as a scaffold first and will be wired
// into status incrementally to avoid behavior regressions.
type Model[T any] struct {
	entries       []Entry[T]
	collapsedDirs map[string]bool
	list          list.Model
	visibleH      int

	search search.Model
	keys   keys.Manager
}

type Result struct {
	Handled          bool
	SelectionChanged bool
	RebuildRequested bool
	OpenSelected     bool
}

func NewModel[T any]() Model[T] {
	return Model[T]{
		collapsedDirs: map[string]bool{},
		search:        search.NewModel(),
		keys:          keys.New(filetreeBindings),
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
	m.list.EnsureSelectionVisible(len(m.entries), m.visibleH)
}

// ScrollOffset returns the current scroll offset of the list.
func (m Model[T]) ScrollOffset() int {
	return m.list.Offset()
}

// SetVisibleHeight stores the visible height used for navigation and scroll.
func (m *Model[T]) SetVisibleHeight(h int) {
	m.visibleH = h
}

// ScrollViewport scrolls the viewport by delta rows, snapping selection into view.
func (m *Model[T]) ScrollViewport(delta int) {
	m.list.ScrollViewport(delta, len(m.entries), m.visibleH)
}

// ScrollPage moves selection and viewport together by delta (vim-style ctrl+d/u).
func (m *Model[T]) ScrollPage(delta int) {
	m.list.ScrollPage(delta, len(m.entries), m.visibleH)
}

func (m Model[T]) selectedEntry() (Entry[T], bool) {
	sel := m.list.Selected()
	if sel < 0 || sel >= len(m.entries) {
		return Entry[T]{}, false
	}
	return m.entries[sel], true
}

func (m Model[T]) CollapsedDirs() map[string]bool {
	out := make(map[string]bool, len(m.collapsedDirs))
	maps.Copy(out, m.collapsedDirs)
	return out
}

func (m *Model[T]) SetCollapsedDirs(dirs map[string]bool) {
	m.collapsedDirs = make(map[string]bool, len(dirs))
	maps.Copy(m.collapsedDirs, dirs)
}

func (m *Model[T]) Search() *search.Model {
	return &m.search
}

func (m Model[T]) ComputeSearchMatches(query string, text func(Entry[T]) string) []search.Match {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []search.Match{}
	}

	var matches []search.Match
	for i, entry := range m.entries {
		if strings.Contains(strings.ToLower(text(entry)), q) {
			matches = append(matches, search.Match{Index: i})
		}
	}
	return matches
}

func (m *Model[T]) RecomputeSearchMatches(text func(Entry[T]) string) {
	m.search.SetMatches(m.ComputeSearchMatches(m.search.Query(), text))
}

func (m *Model[T]) ApplyPassiveSearch(query string, text func(Entry[T]) string) {
	m.search.SetPassiveResults(query, m.ComputeSearchMatches(query, text))
}

func (m *Model[T]) FocusCurrentSearchMatch() bool {
	match, ok := m.search.Match(m.search.Cursor())
	if !ok || match.Index < 0 || match.Index >= len(m.entries) {
		return false
	}
	prev := m.SelectedIndex()
	m.SetSelectedIndex(match.Index)
	return m.SelectedIndex() != prev
}

func (m Model[T]) SearchMatch(index int) (matched bool, current bool) {
	if !m.search.HasQuery() {
		return false, false
	}
	for i, match := range m.search.Matches() {
		if match.Index == index {
			return true, i == m.search.Cursor()
		}
	}
	return false, false
}

func (m *Model[T]) Keys() *keys.Manager {
	return &m.keys
}

func (m Model[T]) Update(msg tea.Msg) (Model[T], tea.Cmd, Result) {
	prevSelected := m.list.Selected()
	if nextSearch, cmd, result := m.search.Update(msg); result.Handled {
		m.search = nextSearch
		return m, cmd, Result{
			Handled:          true,
			SelectionChanged: m.list.Selected() != prevSelected,
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
				m.list.Navigate(+1, len(m.entries), m.visibleH)
				return m, nil, Result{Handled: true, SelectionChanged: m.list.Selected() != prevSelected}
			case BindingMoveUp:
				m.list.Navigate(-1, len(m.entries), m.visibleH)
				return m, nil, Result{Handled: true, SelectionChanged: m.list.Selected() != prevSelected}
			case BindingCollapse:
				if collapseSelectedDir(m.entries, m.collapsedDirs, m.list.Selected()) {
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
				if entry.Kind == EntryFile {
					return m, nil, Result{Handled: true, OpenSelected: true}
				}
				if expandSelectedDir(m.entries, m.collapsedDirs, m.list.Selected()) {
					return m, nil, Result{Handled: true, RebuildRequested: true}
				}
				if idx, ok := firstChildIndex(m.entries, m.list.Selected()); ok && idx != m.list.Selected() {
					m.list.SetSelected(idx, len(m.entries))
					return m, nil, Result{Handled: true, SelectionChanged: true}
				}
				return m, nil, Result{Handled: true}
			case BindingToggle:
				if toggleDirOnEnter(m.entries, m.collapsedDirs, m.list.Selected()) {
					return m, nil, Result{Handled: true, RebuildRequested: true}
				}
				return m, nil, Result{Handled: true, OpenSelected: true}
			default:
				// parent-level binding — let delegateToFiletree handle via Keys().Process
				return m, nil, Result{}
			}
		}
	}
	return m, nil, Result{}
}

func (m *Model[T]) MoveToAdjacentFile(delta int) bool {
	idx, ok := adjacentFileIndex(m.entries, m.list.Selected(), delta)
	if !ok {
		return false
	}
	m.list.SetSelected(idx, len(m.entries))
	return true
}

func (m *Model[T]) ToggleSelectedDir() bool {
	if !toggleDirOnEnter(m.entries, m.collapsedDirs, m.list.Selected()) {
		return false
	}
	return true
}

func (m *Model[T]) CollapseSelectedDir() bool {
	if collapseSelectedDir(m.entries, m.collapsedDirs, m.list.Selected()) {
		return true
	}
	if idx, ok := parentIndex(m.entries, m.list.Selected()); ok && idx != m.list.Selected() {
		m.list.SetSelected(idx, len(m.entries))
		return true
	}
	return false
}

func (m *Model[T]) ExpandSelectedDir() bool {
	return expandSelectedDir(m.entries, m.collapsedDirs, m.list.Selected())
}

func (m *Model[T]) FocusParent() bool {
	idx, ok := parentIndex(m.entries, m.list.Selected())
	if !ok || idx == m.list.Selected() {
		return false
	}
	m.list.SetSelected(idx, len(m.entries))
	return true
}
