package commit

import "github.com/elentok/gx/ui/list"

func (m *Model) scrollHeader(delta int) {
	visible := m.headerViewportRowsCount()
	total := len(m.headerLines())
	maxOffset := max(0, total-visible)
	m.headerOffset += delta
	if m.headerOffset < 0 {
		m.headerOffset = 0
	}
	if m.headerOffset > maxOffset {
		m.headerOffset = maxOffset
	}
}

func (m *Model) scrollHeaderPage(direction int) {
	m.scrollHeader(direction * list.DefaultScroll)
}
