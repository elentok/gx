package status

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diff"
	"github.com/elentok/gx/ui/explorer"
)

type statusExplorerFileSelection struct {
	explorer.FileSelection
	stageFile git.StageFileStatus
}

type statusExplorerDiffSelection struct {
	file statusExplorerFileSelection
}

func (m Model) diffEmptyMessage() string {
	return "No file selected"
}

func (m Model) sectionTitle(section diffSection) string {
	if section == sectionStaged {
		return "Staged"
	}
	return "Unstaged"
}

func (m *Model) sectionState(section diffSection) *sectionState {
	if section == sectionStaged {
		return &m.staged
	}
	return &m.unstaged
}

func (m Model) sectionHasContent(section diffSection) bool {
	var sec sectionState
	if section == sectionStaged {
		sec = m.staged
	} else {
		sec = m.unstaged
	}
	return len(sec.viewLines) > 0 || diff.SectionHasBinaryDiff(sec.parsed)
}

func (m Model) visibleDiffSections() []diffSection {
	sections := make([]diffSection, 0, 2)
	if m.sectionHasContent(sectionUnstaged) {
		sections = append(sections, sectionUnstaged)
	}
	if m.sectionHasContent(sectionStaged) {
		sections = append(sections, sectionStaged)
	}
	return sections
}

func (m Model) explorerCanApplySelection() bool {
	return true
}

func (m Model) explorerCanDiscardSelection() bool {
	return true
}

func (m Model) explorerCanJumpFiles() bool {
	return true
}

func (m Model) explorerCanEditSelection() bool {
	return true
}

func (m Model) explorerCanRunBranchActions() bool {
	return true
}

func (m Model) selectedExplorerFile() (statusExplorerFileSelection, bool) {
	file, ok := m.selectedFile()
	if !ok {
		return statusExplorerFileSelection{}, false
	}
	return statusExplorerFileSelection{
		FileSelection: explorer.FileSelection{
			Path:       file.Path,
			RenameFrom: file.RenameFrom,
			Untracked:  file.IsUntracked(),
		},
		stageFile: file,
	}, true
}

func (m Model) selectedExplorerDiff() (statusExplorerDiffSelection, bool) {
	file, ok := m.selectedExplorerFile()
	if !ok {
		return statusExplorerDiffSelection{}, false
	}
	return statusExplorerDiffSelection{file: file}, true
}
