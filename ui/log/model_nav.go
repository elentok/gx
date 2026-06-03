package log

// SelectRef returns a copy with cursor moved to the commit matching fullHash.
// If rows are not yet loaded, stores the ref to apply on the next reload.
// If no matching row is found, cursor is unchanged.
func (m Model) SelectRef(fullHash string) Model {
	if fullHash == "" {
		return m
	}
	if len(m.rows) == 0 {
		m.pendingFocusRef = fullHash
		return m
	}
	for i := range m.rows {
		if m.rows[i].kind == rowCommit && m.rows[i].commit.FullHash == fullHash {
			m.list.SetSelected(i, len(m.rows))
			m.list.EnsureSelectionVisible(len(m.rows), maxInt(1, m.height-3))
			return m
		}
	}
	return m
}

// SelectedRef returns the full hash of the currently selected commit row,
// or the pending focus ref if rows are not yet loaded.
func (m Model) SelectedRef() string {
	if len(m.rows) == 0 {
		return m.pendingFocusRef
	}
	cursor := m.list.Selected()
	if cursor < 0 || cursor >= len(m.rows) {
		return ""
	}
	if m.rows[cursor].kind != rowCommit {
		return ""
	}
	return m.rows[cursor].commit.FullHash
}

