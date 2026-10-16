package worktrees

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) View() tea.View {
	var content string
	if !m.ready {
		content = "\n  Initializing…"
	} else if m.err != nil {
		content = "\n  Error: " + m.err.Error()
	} else {
		bg := m.normalView()

		if m.pull.IsOpen {
			content = ui.OverlayCenter(bg, m.pull.View(m.width), m.width, m.height)
			return ui.NewMainView(content)
		}

		if m.confirm.IsOpen {
			content = ui.OverlayCenter(bg, m.confirmModalView(), m.width, m.height)
		} else {
			switch m.mode {
			case modeCredentialPrompt:
				content = ui.OverlayCenter(bg, m.credentialModalView(), m.width, m.height)
			case modePushDiverged:
				content = ui.OverlayCenter(bg, m.pushDivergedModalView(), m.width, m.height)
			case modeError:
				content = ui.OverlayCenter(bg, m.errorModalView(), m.width, m.height)
			case modeDeleteProgress:
				content = ui.OverlayCenter(bg, m.deleteProgressModalView(), m.width, m.height)
			case modeLogs:
				content = ui.OverlayCenter(bg, m.logsModalView(), m.width, m.height)
			case modeHelp:
				content = ui.OverlayCenter(bg, m.helpModalView(), m.width, m.height)
			case modeYank:
				content = ui.OverlayCenter(bg, m.yankModalView(), m.width, m.height)
			case modeTerminalMenu:
				content = ui.OverlayCenter(bg, m.terminalMenuModalView(), m.width, m.height)
			case modeRename, modeClone, modeNew, modeNewAndOpen:
				overlay := m.textInputOverlayView()
				y := m.settings.InputModalBottom.ResolveY(m.height, lipgloss.Height(overlay))
				content = ui.OverlayBottomCenter(bg, overlay, m.width, y)
			default:
				content = bg
				if prefix := m.keyManager.Prefix(); len(prefix) > 0 {
					hints := ui.ChordBindingsFromHints(m.keyManager.ChordHints())
					if len(hints) > 0 {
						content = ui.OverlayBottomRight(content, ui.RenderChordOverlay(prefix[0], hints), m.width, m.height)
					}
				}
				if m.search.Mode() == search.SearchModeInput {
					overlayW := m.searchOverlayWidth()
					m.search.SetWidth(overlayW)
					overlay := m.search.View()
					y := m.settings.InputModalBottom.ResolveY(m.height, lipgloss.Height(overlay))
					content = ui.OverlayBottomCenter(content, overlay, m.width, y)
				}
			}
		}
	}

	return ui.NewMainView(content)
}

func (m Model) searchOverlayWidth() int {
	max := m.width * 80 / 100
	if search.DESIRED_WIDTH < max {
		return search.DESIRED_WIDTH
	}
	return max
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
	case modeNewAndOpen:
		title = "New Worktree"
		body = inputView
	case modeSearch:
		title = "Search"
		body = inputView
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

// normalView renders the worktrees table, preview panel, and status bar.
func (m Model) normalView() string {
	h := m.contentHeight()
	tableW, previewW := m.splitWidth()
	tableH, previewH := m.splitHeight(h)

	// The table is always the focused, selection-driving pane (there's no
	// standalone-table state and no focus toggle into the details viewport),
	// so it always renders active and always in sidebar mode.
	tableView := m.renderWorktreesPanel(tableW, tableH, "Worktrees", strings.Split(tableView(m.table), "\n"), true, true)
	previewView := m.renderWorktreesPanel(previewW, previewH, "Details", strings.Split(m.viewport.View(), "\n"), false, false)

	var content string
	if m.useStackedLayout() {
		seam := ui.RenderSeamRow(tableW, ui.SeamColor)
		content = lipgloss.JoinVertical(lipgloss.Left, tableView, seam, previewView)
	} else {
		seam := ui.RenderSeamColumn(tableH, ui.SeamColor)
		content = lipgloss.JoinHorizontal(lipgloss.Top, tableView, seam, previewView)
	}
	return content
}

// renderWorktreesPanel renders one panel (table or details) of the normal
// view, following the frame-free pattern in ui/log/list_panel.go and
// ui/status/view_chrome.go. The active panel (always the table) carries the
// page's busy and paste state on its header's right side.
func (m Model) renderWorktreesPanel(width, height int, title string, lines []string, active, sidebar bool) string {
	accent := color.Color(nil)
	if active {
		accent = ui.ColorBlue
	}
	opts := ui.PanelOptionsFor(width, height, title, "", lines, active, ui.ColorBlue, accent, sidebar)
	if active {
		avail := width - 2*opts.PaddingX - ansi.StringWidth(title) - 1
		opts.RightTitle = m.headerStatus(avail, opts.HeaderBackground)
	}
	return ui.RenderPanel(opts)
}

// headerStatus renders the busy spinner or the paste chip, each followed by
// its dim text, into avail cells. Paste mode wins over a running job. The
// text gives way first; once it is gone, RenderPanel truncates the title, so
// the spinner and the chip never drop.
func (m Model) headerStatus(avail int, bg color.Color) string {
	dim := ui.StyleDim.Background(bg)
	var fixed, text string
	switch {
	case m.mode == modePaste && m.clipboard != nil:
		fixed = ui.RenderBadge("PASTE", ui.BadgeVariantBlue, false, true)
		text = fmt.Sprintf("%d files from %s", len(m.clipboard.files), m.clipboard.srcName)
	case m.spinnerActive:
		fixed = dim.Render(strings.TrimSpace(m.spinner.View()))
		text = m.spinnerLabel
	default:
		return ""
	}
	textW := avail - ansi.StringWidth(fixed) - 1
	if textW < 1 {
		return fixed
	}
	return fixed + dim.Render(" "+ansi.Truncate(text, textW, "…"))
}
