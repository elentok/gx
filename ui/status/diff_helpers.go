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

func (m Model) selectedStatusFile() (statusDiffFileSelection, bool) {
	file, ok := m.filetree.SelectedFile()
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
