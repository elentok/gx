package status

func (m *Model) jumpFiletreeTop() {
	if m.filetree.jumpTop() {
		m.onFiletreeSelectionChanged()
	}
}

func (m *Model) jumpFiletreeBottom() {
	if m.filetree.jumpBottom() {
		m.onFiletreeSelectionChanged()
	}
}

func (m *Model) onFiletreeSelectionChanged() {
	m.overlay.MarkDirty()
}
