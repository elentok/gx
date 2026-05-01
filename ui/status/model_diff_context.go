package status

import "fmt"

func (m Model) currentDiffContextLines() int {
	if m.diffContextLines < 1 {
		return 1
	}
	if m.diffContextLines > 20 {
		return 20
	}
	return m.diffContextLines
}

func (m *Model) adjustDiffContextLines(delta int) {
	next := m.currentDiffContextLines() + delta
	if next < 1 {
		next = 1
	}
	if next > 20 {
		next = 20
	}
	if next == m.currentDiffContextLines() {
		m.setStatus(fmt.Sprintf("diff context: %d", next))
		return
	}
	m.diffContextLines = next
	m.setStatus(fmt.Sprintf("diff context: %d", next))
	m.reloadDiffsForSelection()
	m.syncDiffViewports()
	if m.focus == focusDiff {
		m.ensureActiveVisible(m.currentSection())
	}
}
