package status

func (m *Model) moveActive(delta int) {
	if !m.diffarea.MoveActive(delta) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
	m.diffarea.ActiveSectionModel().EnsureActiveVisible(m.diffarea.NavMode())
}

func (m *Model) jumpDiffTop() {
	if !m.diffarea.JumpTop() {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}

func (m *Model) jumpDiffBottom() {
	if !m.diffarea.JumpBottom() {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}
