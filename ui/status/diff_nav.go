package status

func (m *Model) moveActive(delta int) {
	if !m.diff.MoveActive(delta) {
		return
	}
	m.syncSearchCursorFromDiffFocus()
	m.diff.ActiveSectionModel().EnsureActiveVisible(m.diff.NavMode())
}

func (m *Model) jumpDiffTop() {
	if !m.diff.JumpTop() {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}

func (m *Model) jumpDiffBottom() {
	if !m.diff.JumpBottom() {
		return
	}
	m.syncSearchCursorFromDiffFocus()
}
