package log

// SelectRef returns a copy with cursor moved to the commit matching fullHash.
// If no matching row is found, cursor is unchanged.
func (m Model) SelectRef(fullHash string) Model {
	if fullHash == "" {
		return m
	}
	for i := range m.rows {
		if m.rows[i].kind == rowCommit && m.rows[i].commit.FullHash == fullHash {
			m.cursor = i
			return m
		}
	}
	return m
}

// SelectedRef returns the full hash of the currently selected commit row.
func (m Model) SelectedRef() string {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return ""
	}
	if m.rows[m.cursor].kind != rowCommit {
		return ""
	}
	return m.rows[m.cursor].commit.FullHash
}

