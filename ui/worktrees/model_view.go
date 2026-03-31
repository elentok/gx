package worktrees

import (
	"fmt"

	"gx/ui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) View() tea.View {
	var content string
	if !m.ready {
		content = "\n  Initializing…"
	} else if m.err != nil {
		content = "\n  Error: " + m.err.Error()
	} else {
		bg := m.normalView()

		switch m.mode {
		case modeConfirm:
			content = overlayModal(bg, m.confirmModalView(), m.width, m.height)
		case modePushDiverged:
			content = overlayModal(bg, m.pushDivergedModalView(), m.width, m.height)
		case modeError:
			content = overlayModal(bg, m.errorModalView(), m.width, m.height)
		case modeLogs:
			content = overlayModal(bg, m.logsModalView(), m.width, m.height)
		case modeYank:
			content = overlayModal(bg, m.yankModalView(), m.width, m.height)
		default:
			content = bg
		}
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// overlayModal centers modal over bg using placeOverlay.
func overlayModal(bg, modal string, screenW, screenH int) string {
	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	x := (screenW - modalW) / 2
	y := (screenH - modalH) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return placeOverlay(bg, modal, x, y)
}

// normalView renders the worktrees table, sidebar, and status bar.
func (m Model) normalView() string {
	h := m.contentHeight()
	tableW, sidebarW := m.splitWidth()
	tableH, sidebarH := m.splitHeight(h)

	tableView := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.ColorBorder).
		Width(tableW).
		Height(tableH).
		Render(tableView(m.table))

	sidebarView := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.ColorBorder).
		Width(sidebarW).
		Height(sidebarH).
		Render(m.viewport.View())

	var content string
	if m.useStackedLayout() {
		content = lipgloss.JoinVertical(lipgloss.Left, tableView, sidebarView)
	} else {
		content = lipgloss.JoinHorizontal(lipgloss.Top, tableView, sidebarView)
	}
	return lipgloss.JoinVertical(lipgloss.Left, content, m.statusBarView())
}

// statusBarView renders the 1-line bar at the bottom of the screen.
func (m Model) statusBarView() string {
	switch m.mode {
	case modeError:
		return ""
	case modeRename:
		return m.renameView()
	case modeClone:
		return m.cloneView()
	case modeNew, modeNewTmuxSession, modeNewTmuxWindow:
		return m.newView()
	case modeSearch:
		return m.searchView()
	default:
		if m.mode == modePaste && m.clipboard != nil {
			return ui.StyleDim.Render(fmt.Sprintf("  %d file(s) from %s  ·  j/k navigate · p paste · esc cancel", len(m.clipboard.files), m.clipboard.srcName))
		}
		if m.spinnerActive {
			return "  " + m.spinner.View() + " " + m.spinnerLabel
		}
		if m.statusMsg != "" {
			return "  " + m.statusMsg
		}
		return m.help.View(keys)
	}
}
