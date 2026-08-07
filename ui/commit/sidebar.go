package commit

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/tree"
)

func (m Model) selectedCommitEntry() (tree.Entry[git.CommitFile], bool) {
	entries := m.fileTreeModel.Entries()
	selected := m.fileTreeModel.SelectedIndex()
	if selected < 0 || selected >= len(entries) {
		return tree.Entry[git.CommitFile]{}, false
	}
	return entries[selected], true
}

func (m Model) selectedCommitFile() (git.CommitFile, bool) {
	entry, ok := m.selectedCommitEntry()
	if !ok || entry.HasChildren {
		return git.CommitFile{}, false
	}
	return entry.Value, true
}

func (m *Model) moveToAdjacentFile(delta int) bool {
	if !m.fileTreeModel.MoveToAdjacentFile(delta) {
		return false
	}
	m.refreshDiff()
	if m.focusDiff {
		m.ensureActiveVisible()
	}
	return true
}

func (m *Model) jumpSidebarTop() bool {
	if len(m.fileTreeModel.Entries()) == 0 || m.fileTreeModel.SelectedIndex() == 0 {
		return false
	}
	m.fileTreeModel.SetSelectedIndex(0)
	m.refreshDiff()
	return true
}

func (m *Model) jumpSidebarBottom() bool {
	entries := m.fileTreeModel.Entries()
	if len(entries) == 0 || m.fileTreeModel.SelectedIndex() == len(entries)-1 {
		return false
	}
	m.fileTreeModel.SetSelectedIndex(len(entries) - 1)
	m.refreshDiff()
	return true
}

func (m *Model) scrollSidebarPage(direction int) {
	prev := m.fileTreeModel.SelectedIndex()
	m.fileTreeModel.ScrollPage(direction * list.DefaultScroll)
	if m.fileTreeModel.SelectedIndex() != prev {
		m.refreshDiff()
	}
}

func (m *Model) rebuildCommitFiletree() {
	entries := tree.BuildEntriesFromPaths(
		m.files,
		func(file git.CommitFile) string { return file.Path },
		m.fileTreeModel.CollapsedIDs(),
	)
	m.fileTreeModel.SetEntries(entries)
}
