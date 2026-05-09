package status

// reconcileFileTreeFromStatusState rebuilds filetree view state from status state.
//
// The final setStatusSelection call is required after SetEntries because row
// count/shape may have changed. It reapplies m.selected, clamps it to a valid
// index in fileTreeModel, and writes the clamped value back to m.selected so
// parent and child selection cannot drift.
func (m *Model) reconcileFileTreeFromStatusState() {
	m.fileTreeModel.SetCollapsedDirs(m.collapsedDirs)
	m.fileTreeModel.SetEntries(m.statusRows)
	m.setStatusSelection(m.selected)
}

func (m *Model) setStatusSelection(index int) {
	m.fileTreeModel.SetSelectedIndex(index)
	m.selected = m.fileTreeModel.SelectedIndex()
}
