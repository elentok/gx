package worktrees

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/ui"

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
		case modeHelp:
			content = ui.OverlayCenter(bg, m.helpModalView(), m.width, m.height)
		case modeYank:
			content = ui.OverlayCenter(bg, m.yankModalView(), m.width, m.height)
		case modeRename, modeClone, modeNew, modeNewTmuxSession, modeNewTmuxWindow, modeSearch:
			overlay := m.textInputOverlayView()
			y := m.settings.InputModalBottom.ResolveY(m.height, lipgloss.Height(overlay))
			content = ui.OverlayBottomCenter(bg, overlay, m.width, y)
		default:
			content = bg
		}
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

const textInputOverlayDesiredWidth = 50

func (m Model) textInputOverlayWidth() int {
	max := m.width * 80 / 100
	if textInputOverlayDesiredWidth < max {
		return textInputOverlayDesiredWidth
	}
	return max
}

// textInputOverlayView renders the framed input overlay for text-input modes.
func (m Model) textInputOverlayView() string {
	var title, rightTitle, body string
	outerW := m.textInputOverlayWidth()
	innerW := outerW - 2 - 2 // minus border and padding
	ti := m.textInput
	ti.SetWidth(innerW)
	inputView := ti.View()

	switch m.mode {
	case modeRename:
		title = "Rename Worktree"
		body = inputView
	case modeClone:
		title = "Clone Worktree"
		body = inputView
	case modeNew:
		title = "New Worktree"
		body = inputView
	case modeNewTmuxSession:
		title = "New Worktree + Tmux Session"
		body = inputView
	case modeNewTmuxWindow:
		title = "New Worktree + Tmux Window"
		body = inputView
	case modeSearch:
		title = "Search"
		body = inputView
		if m.searchQuery != "" && len(m.searchMatches) == 0 {
			rightTitle = "no matches"
		} else if len(m.searchMatches) > 0 {
			rightTitle = fmt.Sprintf("%d/%d", m.searchCursor+1, len(m.searchMatches))
		}
	}

	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         title,
		RightTitle:    rightTitle,
		Body:          body,
		Width:         outerW,
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		TitleInBorder: true,
	})
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
		return "  " + ui.StyleHint.Render("? help")
	}
}
