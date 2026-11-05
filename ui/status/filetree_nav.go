package status

func jumpFiletreeTop(m *Model) {
	if m.filetree.jumpTop() {
		onFiletreeSelectionChanged(m)
	}
}

func jumpFiletreeBottom(m *Model) {
	if m.filetree.jumpBottom() {
		onFiletreeSelectionChanged(m)
	}
}

func onFiletreeSelectionChanged(m *Model) {
	m.overlay.MarkDirty()
}
