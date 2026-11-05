package status

import (
	"github.com/elentok/gx/git/stage"
)

type statusDiffFileSelection struct {
	Path       string
	RenameFrom string
	Untracked  bool
	stageFile  stage.StageFileStatus
}

type statusDiffSelection struct {
	file statusDiffFileSelection
}

func (m Model) selectedStatusFile() (statusDiffFileSelection, bool) {
	file, ok := m.filetree.selectedFile()
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
