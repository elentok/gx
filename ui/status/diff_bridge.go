package status

import uidiff "github.com/elentok/gx/ui/diff"

// syncDiffModels keeps the diff child models in sync while status still owns
// the legacy sectionState fields during incremental migration.
func (m *Model) syncDiffModels() {
	m.unstagedDiffModel.SetData(m.unstaged.data)
	m.stagedDiffModel.SetData(m.staged.data)
}

func (m *Model) syncSectionsFromDiffModels() {
	m.unstaged.data = m.unstagedDiffModel.Data()
	m.staged.data = m.stagedDiffModel.Data()
}

func (m *Model) currentDiffModelPtr() *uidiff.Model {
	if m.section == sectionStaged {
		return &m.stagedDiffModel
	}
	return &m.unstagedDiffModel
}

func (m *Model) diffModelForSectionPtr(section diffSection) *uidiff.Model {
	if section == sectionStaged {
		return &m.stagedDiffModel
	}
	return &m.unstagedDiffModel
}

func (m *Model) syncSectionFromDiffModel(section diffSection) {
	model := m.diffModelForSectionPtr(section)
	if section == sectionStaged {
		m.staged.data = model.Data()
		return
	}
	m.unstaged.data = model.Data()
}
