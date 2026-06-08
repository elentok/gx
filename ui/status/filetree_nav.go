package status

import "github.com/elentok/gx/ui/list"

func (m *Model) scrollFiletreePage(direction int) bool {
	if len(m.fileTreeModel.Entries()) == 0 {
		return false
	}
	old := m.fileTreeModel.SelectedIndex()
	m.fileTreeModel.ScrollPage(direction * list.DefaultScroll)
	if m.fileTreeModel.SelectedIndex() == old {
		return false
	}
	m.setStatusSelection(m.fileTreeModel.SelectedIndex())
	return true
}

func (m *Model) jumpFiletreeTop() {
	m.setStatusSelection(m.statusData.listState.Selected())
	if len(m.fileTreeModel.Entries()) == 0 {
		return
	}
	if m.fileTreeModel.SelectedIndex() == 0 {
		return
	}
	m.setStatusSelection(0)
	m.onFiletreeSelectionChanged()
}

func (m *Model) jumpFiletreeBottom() {
	m.setStatusSelection(m.statusData.listState.Selected())
	entryCount := len(m.fileTreeModel.Entries())
	if entryCount == 0 {
		return
	}
	if m.fileTreeModel.SelectedIndex() == entryCount-1 {
		return
	}
	m.setStatusSelection(entryCount - 1)
	m.onFiletreeSelectionChanged()
}

func (m *Model) onFiletreeSelectionChanged() {
	m.imageDiff.dirty = true
}
