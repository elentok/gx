package filetree

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/keybindings"
	"github.com/elentok/gx/ui/search"
	"maps"
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

const (
	filetreeCat = "Filetree"

	BindingMoveDown   keybindings.BindingID = "move-down"
	BindingMoveUp     keybindings.BindingID = "move-up"
	BindingCollapse   keybindings.BindingID = "collapse"
	BindingExpand     keybindings.BindingID = "expand"
	BindingToggle     keybindings.BindingID = "toggle"
	BindingSearch     keybindings.BindingID = "search"
	BindingSearchNext  keybindings.BindingID = "search-next"
	BindingSearchPrev  keybindings.BindingID = "search-prev"
	BindingBack        keybindings.BindingID = "back"
	BindingPageDown    keybindings.BindingID = "page-down"
	BindingPageUp      keybindings.BindingID = "page-up"
	BindingToggleStage keybindings.BindingID = "toggle-stage"
	BindingDiscard     keybindings.BindingID = "discard"
)

var filetreeBindings = []keybindings.Binding{
	{ID: BindingMoveDown, Seq: []string{"j"}, Categories: []string{filetreeCat}, Title: "move down", Display: "↓/j"},
	{ID: BindingMoveUp, Seq: []string{"k"}, Categories: []string{filetreeCat}, Title: "move up", Display: "↑/k"},
	{ID: BindingCollapse, Seq: []string{"h"}, Categories: []string{filetreeCat}, Title: "collapse / go to parent", Display: "h/←"},
	{ID: BindingExpand, Seq: []string{"l"}, Categories: []string{filetreeCat}, Title: "expand / open", Display: "l/→"},
	{ID: BindingToggle, Seq: []string{"enter"}, Categories: []string{filetreeCat}, Title: "open / toggle dir"},
	{ID: BindingSearch, Seq: []string{"/"}, Categories: []string{filetreeCat}, Title: "search"},
	{ID: BindingSearchNext, Seq: []string{"n"}, Categories: []string{filetreeCat}, Title: "next match"},
	{ID: BindingSearchPrev, Seq: []string{"N"}, Categories: []string{filetreeCat}, Title: "prev match"},
	{ID: BindingMoveDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
	{ID: BindingMoveUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
	{ID: BindingCollapse, Seq: []string{"left"}, Categories: []string{}, Title: ""},
	{ID: BindingExpand, Seq: []string{"right"}, Categories: []string{}, Title: ""},
	{ID: BindingBack, Seq: []string{"q"}, Categories: []string{filetreeCat}, Title: "back"},
	{ID: BindingBack, Seq: []string{"esc"}, Categories: []string{}, Title: ""},
	{ID: BindingPageDown, Seq: []string{"ctrl+d"}, Categories: []string{filetreeCat}, Title: "scroll page down"},
	{ID: BindingPageUp, Seq: []string{"ctrl+u"}, Categories: []string{filetreeCat}, Title: "scroll page up"},
	{ID: BindingToggleStage, Seq: []string{"space"}, Categories: []string{filetreeCat}, Title: "toggle stage"},
	{ID: BindingDiscard, Seq: []string{"d"}, Categories: []string{filetreeCat}, Title: "discard"},
}

// Model owns the status/filetree list state and its local search state.
//
// Note: this is intentionally introduced as a scaffold first and will be wired
// into status incrementally to avoid behavior regressions.
type Model[T any] struct {
	entries       []Entry[T]
	collapsedDirs map[string]bool
	selected      int

	search search.Model
	keys   keybindings.Manager
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
		keys:          keybindings.New(filetreeBindings),
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
	if len(m.entries) == 0 {
		m.selected = 0
		return
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.entries) {
		m.selected = len(m.entries) - 1
	}
}

func (m Model[T]) SelectedIndex() int {
	return m.selected
}

func (m *Model[T]) SetSelectedIndex(index int) {
	if len(m.entries) == 0 {
		m.selected = 0
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(m.entries) {
		index = len(m.entries) - 1
	}
	m.selected = index
}

func (m Model[T]) selectedEntry() (Entry[T], bool) {
	if m.selected < 0 || m.selected >= len(m.entries) {
		return Entry[T]{}, false
	}
	return m.entries[m.selected], true
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

func (m *Model[T]) Keys() *keybindings.Manager {
	return &m.keys
}

func (m Model[T]) Update(msg tea.Msg) (Model[T], tea.Cmd, Result) {
	prevSelected := m.selected
	if nextSearch, cmd, result := m.search.Update(msg); result.Handled {
		m.search = nextSearch
		return m, cmd, Result{
			Handled:          true,
			SelectionChanged: m.selected != prevSelected,
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
				if m.selected < len(m.entries)-1 {
					m.selected++
				}
				return m, nil, Result{Handled: true, SelectionChanged: m.selected != prevSelected}
			case BindingMoveUp:
				if m.selected > 0 {
					m.selected--
				}
				return m, nil, Result{Handled: true, SelectionChanged: m.selected != prevSelected}
			case BindingCollapse:
				if collapseSelectedDir(m.entries, m.collapsedDirs, m.selected) {
					return m, nil, Result{Handled: true, RebuildRequested: true}
				}
				if idx, ok := parentIndex(m.entries, m.selected); ok && idx != m.selected {
					m.selected = idx
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
				if expandSelectedDir(m.entries, m.collapsedDirs, m.selected) {
					return m, nil, Result{Handled: true, RebuildRequested: true}
				}
				if idx, ok := firstChildIndex(m.entries, m.selected); ok && idx != m.selected {
					m.selected = idx
					return m, nil, Result{Handled: true, SelectionChanged: true}
				}
				return m, nil, Result{Handled: true}
			case BindingToggle:
				if toggleDirOnEnter(m.entries, m.collapsedDirs, m.selected) {
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
	idx, ok := adjacentFileIndex(m.entries, m.selected, delta)
	if !ok {
		return false
	}
	m.selected = idx
	return true
}

func (m *Model[T]) ToggleSelectedDir() bool {
	if !toggleDirOnEnter(m.entries, m.collapsedDirs, m.selected) {
		return false
	}
	return true
}

func (m *Model[T]) CollapseSelectedDir() bool {
	if collapseSelectedDir(m.entries, m.collapsedDirs, m.selected) {
		return true
	}
	if idx, ok := parentIndex(m.entries, m.selected); ok && idx != m.selected {
		m.selected = idx
		return true
	}
	return false
}

func (m *Model[T]) ExpandSelectedDir() bool {
	return expandSelectedDir(m.entries, m.collapsedDirs, m.selected)
}

func (m *Model[T]) FocusParent() bool {
	idx, ok := parentIndex(m.entries, m.selected)
	if !ok || idx == m.selected {
		return false
	}
	m.selected = idx
	return true
}
