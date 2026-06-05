package log

// SelectRef returns a copy with cursor moved to the commit matching fullHash.
// If rows are not yet loaded, stores the ref to apply on the next reload.
// If no matching row is found, cursor is unchanged.
func (m Model) SelectRef(fullHash string) Model {
	if fullHash == "" {
		return m
	}
	rows := m.listPanel.Rows()
	if len(rows) == 0 {
		m.pendingFocusRef = fullHash
		return m
	}
	for i := range rows {
		if rows[i].kind == rowCommit && rows[i].commit.FullHash == fullHash {
			m.listPanel = m.listPanel.SetSelected(i)
			return m
		}
	}
	return m
}

// SelectedRef returns the full hash of the currently selected commit row,
// or the pending focus ref if rows are not yet loaded.
func (m Model) SelectedRef() string {
	rows := m.listPanel.Rows()
	if len(rows) == 0 {
		return m.pendingFocusRef
	}
	return m.listPanel.SelectedRef()
}
