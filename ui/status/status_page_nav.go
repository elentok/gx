package status

func (m *Model) scrollStatusPage(direction int) bool {
	if len(m.statusEntries) == 0 {
		return false
	}
	old := m.selected
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	statusH, _ := m.splitHeight(mainH)
	visible := maxInt(1, (statusH-2)/2)
	if direction > 0 {
		m.selected += visible
	} else {
		m.selected -= visible
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.statusEntries) {
		m.selected = len(m.statusEntries) - 1
	}
	if m.selected == old {
		return false
	}
	m.onStatusSelectionChanged()
	return true
}

func (m *Model) jumpStatusTop() {
	if len(m.statusEntries) == 0 {
		return
	}
	if m.selected == 0 {
		return
	}
	m.selected = 0
	m.onStatusSelectionChanged()
}

func (m *Model) jumpStatusBottom() {
	if len(m.statusEntries) == 0 {
		return
	}
	if m.selected == len(m.statusEntries)-1 {
		return
	}
	m.selected = len(m.statusEntries) - 1
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
