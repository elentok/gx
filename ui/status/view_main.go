package stage

import (
	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) View() tea.View {
	if !m.ready {
		v := tea.NewView("\n  Loading status UI…")
		v.AltScreen = true
		return v
	}

	if m.err != nil {
		v := tea.NewView("\n  Error: " + m.err.Error())
		v.AltScreen = true
		return v
	}

	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}

	statusW, diffW := m.splitWidth()
	statusH, diffH := m.splitHeight(mainH)

	var body string
	if m.diffFullscreen && m.focus == focusDiff {
		body = m.renderDiffPane(m.width, mainH)
	} else {
		statusPanel := m.renderLeftPane(statusW, statusH)
		diffPanel := m.renderDiffPane(diffW, diffH)
		if m.useStackedLayout() {
			body = lipgloss.JoinVertical(lipgloss.Left, statusPanel, diffPanel)
		} else {
			body = lipgloss.JoinHorizontal(lipgloss.Top, statusPanel, diffPanel)
		}
	}

	footer := m.helpLine()
	out := lipgloss.JoinVertical(lipgloss.Left, body, footer)
	if m.searchMode == searchModeInput {
		overlay := m.searchInputOverlayView()
		y := m.settings.InputModalBottom.ResolveY(m.height, lipgloss.Height(overlay))
		out = ui.OverlayBottomCenter(out, overlay, m.width, y)
	}
	if m.credentialOpen {
		out = ui.OverlayCenter(out, m.credentialModalView(), m.width, m.height)
	} else if m.runningOpen {
		out = ui.OverlayCenter(out, m.runningModalView(), m.width, m.height)
	} else if m.outputOpen {
		out = ui.OverlayCenter(out, m.outputModalView(), m.width, m.height)
	} else if m.confirmOpen {
		out = ui.OverlayCenter(out, m.confirmModalView(), m.width, m.height)
	} else if m.errorOpen {
		out = ui.OverlayCenter(out, m.errorModalView(), m.width, m.height)
	} else if m.helpOpen {
		out = ui.OverlayCenter(out, m.helpModalView(), m.width, m.height)
	}
	v := tea.NewView(out)
	v.AltScreen = true
	v.ReportFocus = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *Model) showGitError(err error) {
	if err == nil {
		return
	}
	m.setStatus("git command failed")
	vpW := m.width * 2 / 3
	if vpW < 44 {
		vpW = 44
	}
	if vpW > 96 {
		vpW = 96
	}
	vpH := m.height/2 - 6
	if vpH < 4 {
		vpH = 4
	}
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent(err.Error())
	m.errorVP = vp
	m.errorOpen = true
}
