package status

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) jumpToTop() {
	if m.focus == focusStatus {
		m.jumpStatusTop()
		return
	}
	m.jumpDiffTop()
}

func (m *Model) jumpToBottom() {
	if m.focus == focusStatus {
		m.jumpStatusBottom()
		return
	}
	m.jumpDiffBottom()
}

func (m *Model) scheduleDiffReload() tea.Cmd {
	m.diffReloadSeq++
	seq := m.diffReloadSeq
	return tea.Tick(statusDiffReloadDebounce, func(time.Time) tea.Msg {
		return diffReloadMsg{seq: seq}
	})
}
