package status

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/explorer"
)

func (m Model) selectedStatusEntry() (statusEntry, bool) {
	if m.selected < 0 || m.selected >= len(m.statusEntries) {
		return statusEntry{}, false
	}
	return m.statusEntries[m.selected], true
}

func (m Model) selectedFile() (git.StageFileStatus, bool) {
	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind != statusEntryFile {
		return git.StageFileStatus{}, false
	}
	return entry.File, true
}

func (m *Model) reload(preservePath string) {
	m.reloadBranchState()
	m.reloadFileList(preservePath)
	m.reloadDiffsForSelection()
}

func (m *Model) reloadFileList(preservePath string) {
	files, err := git.ListStageFiles(m.worktreeRoot)
	if err != nil {
		m.err = err
		m.files = nil
		m.statusEntries = nil
		m.unstaged = newSectionState()
		m.staged = newSectionState()
		return
	}
	m.err = nil
	m.files = files
	m.statusEntries = buildStatusEntries(m.files, m.collapsedDirs)
	if strings.TrimSpace(m.searchQuery) != "" && m.searchScope == searchScopeStatus {
		m.recomputeSearchMatches()
	}

	if len(m.statusEntries) == 0 {
		m.selected = 0
		m.activeFilePath = ""
		m.unstaged = newSectionState()
		m.staged = newSectionState()
		m.focus = focusStatus
		return
	}

	targetPath := preservePath
	if targetPath == "" && m.initialPath != "" {
		targetPath = m.initialPath
		m.initialPath = ""
	}

	if targetPath != "" {
		for i, entry := range m.statusEntries {
			if entry.Path == targetPath {
				m.selected = i
				break
			}
		}
	}
	if m.selected >= len(m.statusEntries) {
		m.selected = len(m.statusEntries) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m *Model) reloadBranchState() {
	m.branchName = ""
	m.branchBaseRef = ""
	m.branchSync = git.SyncStatus{Name: git.StatusUnknown}

	branch, err := git.CurrentBranch(m.worktreeRoot)
	if err != nil || strings.TrimSpace(branch) == "" || strings.TrimSpace(branch) == "HEAD" {
		return
	}
	m.branchName = strings.TrimSpace(branch)
	m.branchBaseRef = git.UpstreamBranch(m.worktreeRoot, m.branchName)
	if m.branchBaseRef == "" {
		return
	}
	sync, err := git.BranchSyncStatusAgainstRef(m.worktreeRoot, m.branchName, m.branchBaseRef)
	if err != nil {
		return
	}
	m.branchSync = sync
}

func (m *Model) toggleDirOnEnter() bool {
	if !explorer.FileTreeToggleDirOnEnter(m.statusFileTreeRows(), m.collapsedDirs, m.selected) {
		return false
	}
	m.statusEntries = buildStatusEntries(m.files, m.collapsedDirs)
	if m.selected >= len(m.statusEntries) {
		m.selected = len(m.statusEntries) - 1
	}
	return true
}

func (m *Model) collapseSelectedDir() {
	if !explorer.FileTreeCollapseSelectedDir(m.statusFileTreeRows(), m.collapsedDirs, m.selected) {
		return
	}
	m.statusEntries = buildStatusEntries(m.files, m.collapsedDirs)
	if m.selected >= len(m.statusEntries) {
		m.selected = len(m.statusEntries) - 1
	}
}

func (m *Model) focusParentInStatus() bool {
	rows := m.statusFileTreeRows()
	idx, ok := explorer.FileTreeParentIndex(rows, m.selected)
	if !ok || m.selected == idx {
		return false
	}
	m.selected = idx
	m.onStatusSelectionChanged()
	return true
}

func (m *Model) expandSelectedDir() {
	if !explorer.FileTreeExpandSelectedDir(m.statusFileTreeRows(), m.collapsedDirs, m.selected) {
		return
	}
	m.statusEntries = buildStatusEntries(m.files, m.collapsedDirs)
}

func (m *Model) moveToAdjacentFile(delta int) bool {
	idx, ok := explorer.FileTreeAdjacentFileIndex(m.statusFileTreeRows(), m.selected, delta)
	if !ok {
		return false
	}
	m.selected = idx
	m.onStatusSelectionChanged()
	m.reloadDiffsForSelection()
	if m.focus == focusDiff {
		m.ensureActiveVisible(m.currentSection())
	}
	return true
}

func (m *Model) toggleStageStatusEntry() {
	entry, ok := m.selectedStatusEntry()
	if !ok {
		return
	}

	path := entry.Path
	stageAll := entry.HasUnstaged
	var err error
	if stageAll {
		err = git.StagePath(m.worktreeRoot, path)
	} else if entry.HasStaged {
		err = git.UnstagePath(m.worktreeRoot, path)
	} else {
		return
	}
	if err != nil {
		m.showGitError(err)
		return
	}
	if stageAll {
		m.setStatus("staged " + path)
	} else {
		m.setStatus("unstaged " + path)
	}
	m.reload(path)
}

func (m *Model) openDiscardStatusConfirm() {
	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind != statusEntryFile {
		return
	}

	title := fmt.Sprintf("Discard changes in %s?", entry.Path)
	paths := []string{entry.Path}
	lines := []string{}

	switch {
	case entry.File.IsUntracked():
		lines = append(lines, "This will delete the untracked file.")
	case entry.File.IsRenamed() && entry.File.RenameFrom != "":
		lines = append(lines,
			"This will undo the rename.",
			entry.File.RenameFrom+" -> "+entry.File.Path,
		)
		paths = []string{entry.File.RenameFrom, entry.File.Path}
	case entry.File.IndexStatus == 'A' || entry.File.WorktreeCode == 'A':
		lines = append(lines, "This will delete the new file.")
	case entry.File.IndexStatus == 'D' || entry.File.WorktreeCode == 'D':
		lines = append(lines, "This will restore the deleted file from HEAD.")
	default:
		lines = append(lines, "This will undo all changes in this file.")
	}

	m.openConfirm(title, lines, confirmDiscardStatus, "", "")
	m.confirmDiscardUntracked = entry.File.IsUntracked()
	m.confirmPaths = uniqueNonEmpty(paths)
}

func (m Model) statusFileTreeRows() []explorer.FileTreeRow[git.StageFileStatus] {
	rows := make([]explorer.FileTreeRow[git.StageFileStatus], 0, len(m.statusEntries))
	for _, entry := range m.statusEntries {
		row := explorer.FileTreeRow[git.StageFileStatus]{
			Path:        entry.Path,
			ParentPath:  entry.ParentPath,
			Depth:       entry.Depth,
			DisplayName: entry.DisplayName,
			Expanded:    entry.Expanded,
		}
		if entry.Kind == statusEntryDir {
			row.Kind = explorer.FileTreeRowDir
		} else {
			row.Kind = explorer.FileTreeRowFile
			row.Value = entry.File
		}
		rows = append(rows, row)
	}
	return rows
}
