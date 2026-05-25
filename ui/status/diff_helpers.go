package status

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/status/diffarea"
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

func (m Model) sectionTitle(section diffarea.Section) string {
	if section == diffarea.SectionStaged {
		return "Staged"
	}
	return "Unstaged"
}

func (m Model) sectionHasContent(section diffarea.Section) bool {
	data := m.diffarea.SectionModel(section).Data()
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
