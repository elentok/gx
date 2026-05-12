package status

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"

	tea "charm.land/bubbletea/v2"
)

func (m Model) selectedFiletreeEntry() (statusEntry, bool) {
	if m.statusData.selected < 0 || m.statusData.selected >= len(m.statusData.statusEntries) {
		return statusEntry{}, false
	}
	return m.statusData.statusEntries[m.statusData.selected], true
}

func (m Model) selectedFile() (git.StageFileStatus, bool) {
	entry, ok := m.selectedFiletreeEntry()
	if !ok || entry.Kind != statusEntryFile {
		return git.StageFileStatus{}, false
	}
	return entry.File, true
}

func (m *Model) reload(preservePath string) tea.Cmd {
	m.reloadBranchState()
	m.reloadFileList(preservePath)
	return m.reloadDiffsForSelection()
}

func (m *Model) reloadFileList(preservePath string) {
	files, err := git.ListStageFiles(m.worktreeRoot)
	if err != nil {
		m.err = err
		m.statusData.files = nil
		m.statusData.statusEntries = nil
		m.statusData.statusRows = nil
		m.reconcileFileTreeFromStatusState()
		m.diff.ResetSections()
		return
	}
	m.err = nil
	m.statusData.files = files
	m.statusData.statusEntries, m.statusData.statusRows = buildStatusEntriesAndRows(m.statusData.files, m.fileTreeModel.CollapsedDirs())
	m.reconcileFileTreeFromStatusState()
	if m.fileTreeModel.Search().HasQuery() {
		matches := m.computeSearchMatches(m.fileTreeModel.Search().Query())
		_ = m.fileTreeModel.Search().SetMatchesAndJump(matches)
	}

	if len(m.statusData.statusEntries) == 0 {
		m.setStatusSelection(0)
		m.activeFilePath = ""
		m.diff.ResetSections()
		m.focus = focusFiletree
		return
	}

	targetPath := preservePath
	if targetPath == "" && m.initialPath != "" {
		targetPath = m.initialPath
		m.initialPath = ""
	}

	if targetPath != "" {
		for i, entry := range m.statusData.statusEntries {
			if entry.Path == targetPath {
				m.setStatusSelection(i)
				break
			}
		}
	}
	m.setStatusSelection(m.statusData.selected)
}

func (m *Model) reloadBranchState() {
	m.statusData.branchName = ""
	m.statusData.branchBaseRef = ""
	m.statusData.branchSync = git.SyncStatus{Name: git.StatusUnknown}

	branch, err := git.CurrentBranch(m.worktreeRoot)
	if err != nil || strings.TrimSpace(branch) == "" || strings.TrimSpace(branch) == "HEAD" {
		return
	}
	m.statusData.branchName = strings.TrimSpace(branch)
	m.statusData.branchBaseRef = git.UpstreamBranch(m.worktreeRoot, m.statusData.branchName)
}

func (m *Model) cmdLoadBranchSync() tea.Cmd {
	if m.statusData.branchName == "" || m.statusData.branchBaseRef == "" {
		return nil
	}
	worktreeRoot := m.worktreeRoot
	branchName := m.statusData.branchName
	branchBaseRef := m.statusData.branchBaseRef
	return func() tea.Msg {
		sync, err := git.BranchSyncStatusAgainstRef(worktreeRoot, branchName, branchBaseRef)
		if err != nil {
			return branchSyncLoadedMsg{branchName: branchName, sync: git.SyncStatus{Name: git.StatusUnknown}}
		}
		return branchSyncLoadedMsg{branchName: branchName, sync: sync}
	}
}

func (m *Model) moveToAdjacentFile(delta int) (bool, tea.Cmd) {
	if !m.fileTreeModel.MoveToAdjacentFile(delta) {
		return false, nil
	}
	m.setStatusSelection(m.fileTreeModel.SelectedIndex())
	m.onFiletreeSelectionChanged()
	cmd := m.reloadDiffsForSelection()
	if m.focus == focusDiff {
		m.diff.ActiveSectionModel().EnsureActiveVisible(m.diff.NavMode())
	}
	return true, cmd
}

func (m *Model) toggleStageStatusEntry() tea.Cmd {
	entry, ok := m.selectedFiletreeEntry()
	if !ok {
		return nil
	}

	path := entry.Path
	stageAll := entry.HasUnstaged
	var err error
	if stageAll {
		err = git.StagePath(m.worktreeRoot, path)
	} else if entry.HasStaged {
		err = git.UnstagePath(m.worktreeRoot, path)
	} else {
		return nil
	}
	if err != nil {
		m.showGitError(err)
		return nil
	}
	if stageAll {
		m.setStatus("staged " + path)
	} else {
		m.setStatus("unstaged " + path)
	}
	return m.reload(path)
}

func (m *Model) openDiscardStatusConfirm() {
	entry, ok := m.selectedFiletreeEntry()
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
