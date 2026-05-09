package status

func (m *Model) scrollStatusPage(direction int) bool {
	m.setStatusSelection(m.selected)
	if len(m.fileTreeModel.Entries()) == 0 {
		return false
	}
	old := m.fileTreeModel.SelectedIndex()
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	statusH, _ := m.splitHeight(mainH)
	visible := maxInt(1, (statusH-2)/2)
	if direction > 0 {
		m.setStatusSelection(old + visible)
	} else {
		m.setStatusSelection(old - visible)
	}
	if m.fileTreeModel.SelectedIndex() == old {
		return false
	}
	m.onStatusSelectionChanged()
	return true
}

func (m *Model) jumpStatusTop() {
	m.setStatusSelection(m.selected)
	if len(m.fileTreeModel.Entries()) == 0 {
		return
	}
	if m.fileTreeModel.SelectedIndex() == 0 {
		return
	}
	m.setStatusSelection(0)
	m.onStatusSelectionChanged()
}

func (m *Model) jumpStatusBottom() {
	m.setStatusSelection(m.selected)
	entryCount := len(m.fileTreeModel.Entries())
	if entryCount == 0 {
		return
	}
	if m.fileTreeModel.SelectedIndex() == entryCount-1 {
		return
	}
	m.setStatusSelection(entryCount - 1)
	m.onStatusSelectionChanged()
}

func (m *Model) onStatusSelectionChanged() {
	entry, ok := m.selectedStatusEntry()
	if !ok || entry.Kind == statusEntryDir {
		m.section = sectionUnstaged
		return
	}
	if entry.File.Path != m.activeFilePath {
		m.section = sectionUnstaged
	}
}
