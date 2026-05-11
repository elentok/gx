package commit

import (
	"github.com/elentok/gx/git"
)

type commitFileEntryKind int

const (
	commitFileEntryFile commitFileEntryKind = iota
	commitFileEntryDir
)

type commitFileEntry struct {
	Kind        commitFileEntryKind
	Path        string
	ParentPath  string
	Depth       int
	DisplayName string
	Expanded    bool
	File        git.CommitFile
}

func buildCommitFileEntries(files []git.CommitFile, collapsed map[string]bool) []commitFileEntry {
	leaves := make([]commitTreeLeaf[git.CommitFile], 0, len(files))
	for i := range files {
		leaves = append(leaves, commitTreeLeaf[git.CommitFile]{
			Path:  files[i].Path,
			Value: files[i],
		})
	}
	rows := buildCommitTreeRows(leaves, collapsed)
	entries := make([]commitFileEntry, 0, len(rows))
	for _, row := range rows {
		entry := commitFileEntry{
			Path:        row.Path,
			ParentPath:  row.ParentPath,
			Depth:       row.Depth,
			DisplayName: row.DisplayName,
			Expanded:    row.Expanded,
		}
		if row.Kind == commitTreeRowDir {
			entry.Kind = commitFileEntryDir
		} else {
			entry.Kind = commitFileEntryFile
			entry.File = row.Value
		}
		entries = append(entries, entry)
	}
	return entries
}

func (m Model) commitFileTreeRows() []commitTreeRow[git.CommitFile] {
	rows := make([]commitTreeRow[git.CommitFile], 0, len(m.fileEntries))
	for _, entry := range m.fileEntries {
		row := commitTreeRow[git.CommitFile]{
			Path:        entry.Path,
			ParentPath:  entry.ParentPath,
			Depth:       entry.Depth,
			DisplayName: entry.DisplayName,
			Expanded:    entry.Expanded,
		}
		if entry.Kind == commitFileEntryDir {
			row.Kind = commitTreeRowDir
		} else {
			row.Kind = commitTreeRowFile
			row.Value = entry.File
		}
		rows = append(rows, row)
	}
	return rows
}

func (m Model) selectedCommitEntry() (commitFileEntry, bool) {
	if m.selected < 0 || m.selected >= len(m.fileEntries) {
		return commitFileEntry{}, false
	}
	return m.fileEntries[m.selected], true
}

func (m Model) selectedCommitFile() (git.CommitFile, bool) {
	entry, ok := m.selectedCommitEntry()
	if !ok || entry.Kind != commitFileEntryFile {
		return git.CommitFile{}, false
	}
	return entry.File, true
}

func (m *Model) toggleDirOnEnter() bool {
	if !commitTreeToggleDirOnEnter(m.commitFileTreeRows(), m.collapsedDirs, m.selected) {
		return false
	}
	m.fileEntries = buildCommitFileEntries(m.files, m.collapsedDirs)
	if m.selected >= len(m.fileEntries) {
		m.selected = len(m.fileEntries) - 1
	}
	if m.searchScope == searchScopeSidebar && m.search.HasQuery() {
		matches := m.computeSearchMatches(m.search.Query())
		m.search.SetMatches(matches)
	}
	return true
}

func (m *Model) collapseSelectedDir() bool {
	if !commitTreeCollapseSelectedDir(m.commitFileTreeRows(), m.collapsedDirs, m.selected) {
		return false
	}
	m.fileEntries = buildCommitFileEntries(m.files, m.collapsedDirs)
	if m.selected >= len(m.fileEntries) {
		m.selected = len(m.fileEntries) - 1
	}
	if m.searchScope == searchScopeSidebar && m.search.HasQuery() {
		matches := m.computeSearchMatches(m.search.Query())
		m.search.SetMatches(matches)
	}
	return true
}

func (m *Model) expandSelectedDir() bool {
	if !commitTreeExpandSelectedDir(m.commitFileTreeRows(), m.collapsedDirs, m.selected) {
		return false
	}
	m.fileEntries = buildCommitFileEntries(m.files, m.collapsedDirs)
	if m.searchScope == searchScopeSidebar && m.search.HasQuery() {
		matches := m.computeSearchMatches(m.search.Query())
		m.search.SetMatches(matches)
	}
	return true
}

func (m *Model) focusParentInSidebar() bool {
	idx, ok := commitTreeParentIndex(m.commitFileTreeRows(), m.selected)
	if !ok || idx == m.selected {
		return false
	}
	m.selected = idx
	return true
}

func (m *Model) moveToAdjacentFile(delta int) bool {
	idx, ok := commitTreeAdjacentFileIndex(m.commitFileTreeRows(), m.selected, delta)
	if !ok {
		return false
	}
	m.selected = idx
	m.refreshDiff()
	if m.focusDiff {
		m.ensureActiveVisible()
	}
	return true
}

func (m *Model) moveSidebar(delta int) bool {
	if len(m.fileEntries) == 0 {
		return false
	}
	next := m.selected + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.fileEntries) {
		next = len(m.fileEntries) - 1
	}
	if next == m.selected {
		return false
	}
	m.selected = next
	m.refreshDiff()
	return true
}

func (m *Model) jumpSidebarTop() bool {
	if len(m.fileEntries) == 0 || m.selected == 0 {
		return false
	}
	m.selected = 0
	m.refreshDiff()
	return true
}

func (m *Model) jumpSidebarBottom() bool {
	if len(m.fileEntries) == 0 || m.selected == len(m.fileEntries)-1 {
		return false
	}
	m.selected = len(m.fileEntries) - 1
	m.refreshDiff()
	return true
}
