package status

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
	return len(sec.viewLines) > 0 || sectionHasBinaryDiff(sec)
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
