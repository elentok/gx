package status

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diff"
)

type explorerFileSelection struct {
	path       string
	renameFrom string
	untracked  bool
	stageFile  git.StageFileStatus
}

type explorerDiffSelection struct {
	file explorerFileSelection
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

func (m Model) selectedExplorerFile() (explorerFileSelection, bool) {
	file, ok := m.selectedFile()
	if !ok {
		return explorerFileSelection{}, false
	}
	return explorerFileSelection{
		path:       file.Path,
		renameFrom: file.RenameFrom,
		untracked:  file.IsUntracked(),
		stageFile:  file,
	}, true
}

func (m Model) selectedExplorerDiff() (explorerDiffSelection, bool) {
	file, ok := m.selectedExplorerFile()
	if !ok {
		return explorerDiffSelection{}, false
	}
	return explorerDiffSelection{file: file}, true
}
