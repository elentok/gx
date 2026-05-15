package log

// SelectRef returns a copy with cursor moved to the commit matching fullHash.
// If no matching row is found, cursor is unchanged.
func (m Model) SelectRef(fullHash string) Model {
	if fullHash == "" {
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

// SelectedRef returns the full hash of the currently selected commit row.
func (m Model) SelectedRef() string {
	cursor := m.list.Selected()
	if cursor < 0 || cursor >= len(m.rows) {
		return ""
	}
	if m.rows[cursor].kind != rowCommit {
		return ""
	}
	return m.rows[cursor].commit.FullHash
}

