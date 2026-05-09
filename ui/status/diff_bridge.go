package status

// syncDiffModels keeps the diff child models in sync while status still owns
// the legacy sectionState fields during incremental migration.
func (m *Model) syncDiffModels() {
	m.unstagedDiffModel.SetData(m.unstaged.data)
	m.stagedDiffModel.SetData(m.staged.data)
}
