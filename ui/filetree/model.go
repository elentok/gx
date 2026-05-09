package filetree

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/explorer"
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
}

type RebuildRequestedMsg struct{}
type OpenSelectedMsg struct{}

func NewModel[T any]() Model[T] {
	return Model[T]{
		collapsedDirs: map[string]bool{},
		search:        search.NewModel(),
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

func (m Model[T]) SelectedEntry() (Entry[T], bool) {
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

func (m Model[T]) Search() search.Model {
	return m.search
}

func (m Model[T]) Update(msg tea.Msg) (Model[T], tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "j", "down":
			if m.selected < len(m.entries)-1 {
				m.selected++
			}
			return m, nil
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
			return m, nil
		case "h", "left":
			rows := m.fileTreeRows()
			if idx, ok := explorer.FileTreeParentIndex(rows, m.selected); ok && idx != m.selected {
				m.selected = idx
				return m, nil
			}
			if explorer.FileTreeCollapseSelectedDir(rows, m.collapsedDirs, m.selected) {
				return m, rebuildRequestedCmd()
			}
			return m, nil
		case "l", "right":
			entry, ok := m.SelectedEntry()
			if !ok {
				return m, nil
			}
			if entry.Kind == EntryFile {
				return m, openSelectedCmd()
			}
			if explorer.FileTreeExpandSelectedDir(m.fileTreeRows(), m.collapsedDirs, m.selected) {
				return m, rebuildRequestedCmd()
			}
			return m, nil
		case "enter":
			if explorer.FileTreeToggleDirOnEnter(m.fileTreeRows(), m.collapsedDirs, m.selected) {
				return m, rebuildRequestedCmd()
			}
			return m, openSelectedCmd()
		}
	}
	return m, nil
}

func (m Model[T]) fileTreeRows() []explorer.FileTreeRow[T] {
	rows := make([]explorer.FileTreeRow[T], 0, len(m.entries))
	for _, entry := range m.entries {
		row := explorer.FileTreeRow[T]{
			Path:        entry.Path,
			ParentPath:  entry.ParentPath,
			Depth:       entry.Depth,
			DisplayName: entry.DisplayName,
			Expanded:    entry.Expanded,
			Value:       entry.Value,
		}
		if entry.Kind == EntryDir {
			row.Kind = explorer.FileTreeRowDir
		} else {
			row.Kind = explorer.FileTreeRowFile
		}
		rows = append(rows, row)
	}
	return rows
}

func rebuildRequestedCmd() tea.Cmd {
	return func() tea.Msg { return RebuildRequestedMsg{} }
}

func openSelectedCmd() tea.Cmd {
	return func() tea.Msg { return OpenSelectedMsg{} }
}
