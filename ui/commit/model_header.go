package commit

const commitHeaderMaxRows = 6

func (m *Model) scrollHeader(delta int) {
	visible := m.headerViewportRowsCount()
	total := len(m.headerLines())
	maxOffset := maxInt(0, total-visible)
	m.headerOffset += delta
	if m.headerOffset < 0 {
		m.headerOffset = 0
	}
	if m.headerOffset > maxOffset {
		m.headerOffset = maxOffset
	}
}

func (m *Model) scrollHeaderPage(direction int) {
	step := m.headerViewportRowsCount() / 2
	if step < 1 {
		step = 1
	}
	m.scrollHeader(direction * step)
}

