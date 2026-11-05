package status

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git/stage"
	"github.com/elentok/gx/ui/status/filetree"
	"github.com/elentok/gx/ui/status/stagemodal"
)

func (m *Model) reload(preservePath string) tea.Cmd {
	m.filetree.ReloadBranchState(m.worktreeRoot)
	m.reloadFileList(preservePath)
	return m.reloadDiffsForSelection()
}

func (m *Model) reloadFileList(preservePath string) {
	files, err := stage.ListStageFiles(m.worktreeRoot)
	if err != nil {
		m.err = err
		m.filetree.ClearFiles()
		m.diffarea.ResetSections()
		return
	}
	m.err = nil
	m.filetree.SetFiles(files)
	if m.filetree.Model.Search().HasQuery() {
		matches := m.filetree.ComputeSearchMatches(m.filetree.Model.Search().Query())
		_ = m.filetree.Model.SetSearchMatchesAndJump(matches)
	}

	if len(m.filetree.Data.Entries) == 0 {
		m.filetree.SetSelection(0)
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
		for i, entry := range m.filetree.Data.Entries {
			if entry.Path == targetPath {
				m.filetree.SetSelection(i)
				break
			}
		}
	}
	m.filetree.SetSelection(m.filetree.Data.ListState.Selected())
}

func moveToAdjacentFile(m *Model, delta int) (bool, tea.Cmd) {
	if !m.filetree.MoveToAdjacent(delta) {
		return false, nil
	}
	onFiletreeSelectionChanged(m)
	cmd := m.reloadDiffsForSelection()
	if m.focus == focusDiff {
		m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
	}
	return true, cmd
}

func toggleStageStatusEntry(m *Model) tea.Cmd {
	entry, ok := m.filetree.SelectedEntry()
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

func openDiscardStatusConfirm(m *Model) {
	entry, ok := m.filetree.SelectedEntry()
	if !ok {
		return
	}
	row, ok := m.filetree.SelectedRow()
	if !ok {
		return
	}

	title := fmt.Sprintf("Discard changes in %s?", entry.Path)
	restorePaths := []string{entry.Path}
	deletePaths := []string{}
	lines := []string{}

	switch entry.Kind {
	case filetree.EntryDir:
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
	case filetree.EntryFile:
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

	openConfirmRequest(m, stagemodal.ConfirmRequest{
		Title:            title,
		Lines:            lines,
		Action:           stagemodal.ConfirmDiscardStatus,
		DiscardUntracked: entry.Kind == filetree.EntryFile && entry.File.IsUntracked(),
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
