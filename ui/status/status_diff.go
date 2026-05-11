package status

import (
	"github.com/elentok/gx/git"
)

type statusDiffFileSelection struct {
	Path       string
	RenameFrom string
	Untracked  bool
	stageFile  git.StageFileStatus
}

type statusDiffSelection struct {
	file statusDiffFileSelection
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

func (m Model) sectionHasContent(section diffSection) bool {
	data := m.diff.SectionModel(section).Data()
	return data.HasContent()
}

func (m Model) selectedStatusFile() (statusDiffFileSelection, bool) {
	file, ok := m.selectedFile()
	if !ok {
		return statusDiffFileSelection{}, false
	}
	return statusDiffFileSelection{
		Path:       file.Path,
		RenameFrom: file.RenameFrom,
		Untracked:  file.IsUntracked(),
		stageFile:  file,
	}, true
}

func (m Model) selectedStatusDiff() (statusDiffSelection, bool) {
	file, ok := m.selectedStatusFile()
	if !ok {
		return statusDiffSelection{}, false
	}
	return statusDiffSelection{file: file}, true
}
