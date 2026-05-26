package commit

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/notify"
)

func (m Model) currentDiffContextLines() int {
	if m.diffContextLines < 1 {
		return 1
	}
	if m.diffContextLines > 20 {
		return 20
	}
	return m.diffContextLines
}

func (m *Model) adjustDiffContextLines(delta int) tea.Cmd {
	next := m.currentDiffContextLines() + delta
	if next < 1 {
		next = 1
	}
	if next > 20 {
		next = 20
	}
	if next == m.currentDiffContextLines() {
		return notify.Info(fmt.Sprintf("diff context: %d", next))
	}
	m.diffContextLines = next
	m.refreshDiff()
	return notify.Info(fmt.Sprintf("diff context: %d", next))
}
