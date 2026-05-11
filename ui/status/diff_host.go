package status

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffrender"
	"github.com/elentok/gx/ui/explorer"
)

type statusDiffFileSelection struct {
	explorer.FileSelection
	stageFile git.StageFileStatus
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

func (m *Model) sectionState(section diffSection) *sectionState {
	if section == sectionStaged {
		return &m.staged
	}
	return &m.unstaged
}

func (m Model) sectionHasContent(section diffSection) bool {
	data := m.diffModelForSectionPtr(section).Data()
	return len(data.ViewLines) > 0 || diffrender.SectionHasBinaryDiff(data.Parsed)
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

func (m Model) selectedStatusFile() (statusDiffFileSelection, bool) {
	file, ok := m.selectedFile()
	if !ok {
		return statusDiffFileSelection{}, false
	}
	return statusDiffFileSelection{
		FileSelection: explorer.FileSelection{
			Path:       file.Path,
			RenameFrom: file.RenameFrom,
			Untracked:  file.IsUntracked(),
		},
		stageFile: file,
	}, true
}

func (m Model) selectedStatusDiff() (statusDiffSelection, bool) {
	file, ok := m.selectedStatusFile()
	if !ok {
		return statusDiffSelection{}, false
	}
	return statusDiffSelection{file: file}, true
}

func (m *Model) diffModelForSectionPtr(section diffSection) *diffview.Model {
	if section == sectionStaged {
		return &m.stagedDiffModel
	}
	return &m.unstagedDiffModel
}

func (m *Model) resetDiffSections() {
	m.unstaged = newSectionState()
	m.staged = newSectionState()
	m.diffModelForSectionPtr(sectionUnstaged).SetData(m.unstaged.data)
	m.diffModelForSectionPtr(sectionStaged).SetData(m.staged.data)
}
