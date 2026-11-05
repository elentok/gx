package status

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui/status/stagemodal"
)

func (m *Model) reload(preservePath string) tea.Cmd {
	m.filetree.reloadBranchState(m.worktreeRoot)
	m.reloadFileList(preservePath)
	return m.reloadDiffsForSelection()
}

func (m *Model) reloadFileList(preservePath string) {
	files, err := stage.ListStageFiles(m.worktreeRoot)
	if err != nil {
		m.err = err
		m.filetree.clearFiles()
		m.diffarea.ResetSections()
		return
	}
	m.err = nil
	m.filetree.setFiles(files)
	if m.filetree.model.Search().HasQuery() {
		matches := m.computeSearchMatches(m.filetree.model.Search().Query())
		_ = m.filetree.model.SetSearchMatchesAndJump(matches)
	}

	if len(m.filetree.data.statusEntries) == 0 {
		m.filetree.setSelection(0)
		m.diffLoader.activeFilePath = ""
		m.diffarea.ResetSections()
		m.focus = focusFiletree
		return
	}

	targetPath := preservePath
	if targetPath == "" && m.initialPath != "" {
		targetPath = m.initialPath
		m.initialPath = ""
	}

	if targetPath != "" {
		for i, entry := range m.filetree.data.statusEntries {
			if entry.Path == targetPath {
				m.filetree.setSelection(i)
				break
			}
		}
	}
	m.filetree.setSelection(m.filetree.data.listState.Selected())
}

func (m *Model) moveToAdjacentFile(delta int) (bool, tea.Cmd) {
	if !m.filetree.moveToAdjacent(delta) {
		return false, nil
	}
	m.onFiletreeSelectionChanged()
	cmd := m.reloadDiffsForSelection()
	if m.focus == focusDiff {
		m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
	}
	return true, cmd
}

func (m *Model) toggleStageStatusEntry() tea.Cmd {
	entry, ok := m.filetree.selectedEntry()
	if !ok {
		return nil
	}

	path := entry.Path
	stageAll := entry.HasUnstaged
	var err error
	if stageAll {
		err = stage.StagePath(m.worktreeRoot, path)
	} else if entry.HasStaged {
		err = stage.UnstagePath(m.worktreeRoot, path)
	} else {
		return nil
	}
	if err != nil {
		m.showGitError(err)
		return nil
	}
	return tea.Batch(m.reload(path), statusRepoMutatedCmd())
}

func (m *Model) openDiscardStatusConfirm() {
	entry, ok := m.filetree.selectedEntry()
	if !ok {
		return
	}
	row, ok := m.filetree.selectedRow()
	if !ok {
		return
	}

	title := fmt.Sprintf("Discard changes in %s?", entry.Path)
	restorePaths := []string{entry.Path}
	deletePaths := []string{}
	lines := []string{}

	switch entry.Kind {
	case statusEntryDir:
		restorePaths, deletePaths = statusDiscardPathsForLeaves(row.Leaves)
		hasUntracked := len(deletePaths) > 0
		switch {
		case len(restorePaths) == 0 && hasUntracked:
			lines = append(lines, "This will delete all untracked files in this directory.")
		case hasUntracked:
			lines = append(lines,
				"This will discard all tracked changes in this directory.",
				"Untracked files inside it will be deleted.",
			)
		default:
			lines = append(lines, "This will discard all tracked changes in this directory.")
		}
	case statusEntryFile:
		switch {
		case entry.File.IsUntracked():
			lines = append(lines, "This will delete the untracked file.")
		case entry.File.IsRenamed() && entry.File.RenameFrom != "":
			lines = append(lines,
				"This will undo the rename.",
				entry.File.RenameFrom+" -> "+entry.File.Path,
			)
			restorePaths = []string{entry.File.RenameFrom, entry.File.Path}
		case entry.File.IndexStatus == 'A' || entry.File.WorktreeCode == 'A':
			lines = append(lines, "This will delete the new file.")
		case entry.File.IndexStatus == 'D' || entry.File.WorktreeCode == 'D':
			lines = append(lines, "This will restore the deleted file from HEAD.")
		default:
			lines = append(lines, "This will undo all changes in this file.")
		}
	default:
		return
	}

	m.openConfirmRequest(stagemodal.ConfirmRequest{
		Title:            title,
		Lines:            lines,
		Action:           stagemodal.ConfirmDiscardStatus,
		DiscardUntracked: entry.Kind == statusEntryFile && entry.File.IsUntracked(),
		Paths:            uniqueNonEmpty(restorePaths),
		DeletePaths:      uniqueNonEmpty(deletePaths),
		DisplayPath:      entry.Path,
		PreservePath:     entry.Path,
	})
}

func statusDiscardPathsForLeaves(files []stage.StageFileStatus) (restorePaths, deletePaths []string) {
	for _, file := range files {
		switch {
		case file.IsUntracked():
			deletePaths = append(deletePaths, file.Path)
		case file.IsRenamed() && file.RenameFrom != "":
			restorePaths = append(restorePaths, file.RenameFrom, file.Path)
		default:
			restorePaths = append(restorePaths, file.Path)
		}
	}
	return uniqueNonEmpty(restorePaths), uniqueNonEmpty(deletePaths)
}
