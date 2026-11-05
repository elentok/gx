package status

func (m *Model) jumpToTop() {
	if m.focus == focusFiletree {
		m.jumpFiletreeTop()
	}
}

func (m *Model) jumpToBottom() {
	if m.focus == focusFiletree {
		m.jumpFiletreeBottom()
	}
}
