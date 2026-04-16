package worktrees

import (
	"fmt"
	"strings"

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
			content = ui.OverlayCenter(bg, m.confirmModalView(), m.width, m.height)
		case modeCredentialPrompt:
			content = ui.OverlayCenter(bg, m.credentialModalView(), m.width, m.height)
		case modePushDiverged:
			content = ui.OverlayCenter(bg, m.pushDivergedModalView(), m.width, m.height)
		case modeError:
			content = ui.OverlayCenter(bg, m.errorModalView(), m.width, m.height)
		case modeLogs:
			content = ui.OverlayCenter(bg, m.logsModalView(), m.width, m.height)
		case modeYank:
			content = ui.OverlayCenter(bg, m.yankModalView(), m.width, m.height)
		default:
			content = bg
		}
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// normalView renders the worktrees table, sidebar, and status bar.
func (m Model) normalView() string {
	h := m.contentHeight()
	tableW, sidebarW := m.splitWidth()
	tableH, sidebarH := m.splitHeight(h)

	tableView := ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       tableW,
		Height:      tableH,
		Title:       "Worktrees",
		Lines:       strings.Split(tableView(m.table), "\n"),
		BorderColor: ui.ColorBorder,
		TitleColor:  ui.ColorBlue,
		Background:  ui.ColorBase,
	})

	sidebarView := ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       sidebarW,
		Height:      sidebarH,
		Title:       "Details",
		Lines:       strings.Split(m.viewport.View(), "\n"),
		BorderColor: ui.ColorBorder,
		TitleColor:  ui.ColorBlue,
		Background:  ui.ColorBase,
	})

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
			prefix := ui.StyleDim.Render(fmt.Sprintf("  %d file(s) from %s", len(m.clipboard.files), m.clipboard.srcName))
			return prefix + ui.StyleDim.Render("  ·  ") + ui.RenderInlineBindings(keys.Up, keys.Down, keys.PasteConfirm, keys.PasteCancel)
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
