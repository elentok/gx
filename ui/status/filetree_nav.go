package status

func jumpFiletreeTop(m *Model) {
	if m.filetree.JumpTop() {
		onFiletreeSelectionChanged(m)
	}
}

func jumpFiletreeBottom(m *Model) {
	if m.filetree.JumpBottom() {
		onFiletreeSelectionChanged(m)
	}
}

func onFiletreeSelectionChanged(m *Model) {
	m.overlay.MarkDirty()
}
