package status

// reconcileFileTreeFromStatusState rebuilds filetree view state from status state.
//
// The final setStatusSelection call is required after SetEntries because row
// count/shape may have changed. It reapplies m.page.selected, clamps it to a valid
// index in fileTreeModel, and writes the clamped value back to m.page.selected so
// parent and child selection cannot drift.
func (m *Model) reconcileFileTreeFromStatusState() {
	m.fileTreeModel.SetEntries(m.page.statusRows)
	m.setStatusSelection(m.page.selected)
}

func (m *Model) setStatusSelection(index int) {
	m.fileTreeModel.SetSelectedIndex(index)
	m.page.selected = m.fileTreeModel.SelectedIndex()
}
