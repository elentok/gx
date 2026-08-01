package tickets

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"
)

func (m FlatModel) View() tea.View {
	if !m.ready {
		return ui.NewMainView("\n  Initializing…")
	}
	content := m.normalView()
	if m.previewSearch.Mode() == search.SearchModeInput {
		overlayW := m.searchOverlayWidth()
		s := m.previewSearch
		s.SetWidth(overlayW)
		overlay := s.View()
		y := m.settings.InputModalBottom.ResolveY(m.height, lipgloss.Height(overlay))
		content = ui.OverlayBottomCenter(content, overlay, m.width, y)
	}
	if stack := m.notify.View(); stack != "" {
		content = ui.OverlayTopRightMargin(content, stack, m.width, 1, 1)
	}
	return ui.NewMainView(content)
}

func (m FlatModel) searchOverlayWidth() int {
	maxW := m.width * 80 / 100
	if search.DESIRED_WIDTH < maxW {
		return search.DESIRED_WIDTH
	}
	return maxW
}

func (m FlatModel) normalView() string {
	listW, previewW := m.flatSplitWidth()
	h := m.flatContentHeight()

	listView := m.renderPanel(listW, h, m.titleLine(), "", m.listLines(), m.focus == flatFocusList, true)
	previewView := m.renderPanel(previewW, h, "Preview", m.previewMatchStatus(), m.previewLines(), m.focus == flatFocusPreview, false)

	if m.flatUseStackedLayout() {
		seam := ui.RenderSeamRow(listW, ui.SeamColor)
		return lipgloss.JoinVertical(lipgloss.Left, listView, seam, previewView)
	}
	seam := ui.RenderSeamColumn(h, ui.SeamColor)
	return lipgloss.JoinHorizontal(lipgloss.Top, listView, seam, previewView)
}

func (m FlatModel) renderPanel(width, height int, title, rightTitle string, lines []string, active, sidebar bool) string {
	return ui.RenderPanel(ui.PanelOptionsFor(width, height, title, rightTitle, lines, active, ui.ColorBlue, nil, sidebar))
}

// listLines renders the flat ticket list: no epic header row, no
// Open/Closed grouping — every row is one of the epic's tickets, ordered by
// sortedTickets and using the same status icon/dimming convention as
// ui/tickets' own sidebar (renderTicketRow/statusIconAndStyle).
func (m FlatModel) listLines() []string {
	if !m.loaded {
		return []string{ui.StyleDim.Render("  loading…")}
	}
	if !m.found {
		return []string{ui.StyleMuted.Render(fmt.Sprintf("  epic %q not found", m.epicName))}
	}
	if len(m.ordered) == 0 {
		return []string{ui.StyleMuted.Render("  no tickets")}
	}
	lines := make([]string, len(m.ordered))
	for i, t := range m.ordered {
		line := m.renderFlatTicketRow(t)
		if i == m.selected {
			line = ui.RenderRowHighlight(line)
		}
		lines[i] = line
	}
	return lines
}

func (m FlatModel) renderFlatTicketRow(t tickets.Ticket) string {
	status := m.epic.RenderedStatus(t)
	icon, style := statusIconAndStyle(m.icons(), status)

	title := fmt.Sprintf("%d %s", t.Number, t.Title)
	titleStyle := lipgloss.NewStyle()
	if status == tickets.StatusDone {
		titleStyle = statusDoneStyle
	}

	line := "  " + style.Render(icon) + " " + titleStyle.Render(title)
	if suffix := blockedBySuffix(m.epic, t, status); suffix != "" {
		line += " " + blockedBySuffixStyle.Render(suffix)
	}
	return line
}

// previewContent builds the selected ticket's preview, mirroring
// Model.previewContent's ticket-row branch (preview.go) — this flat model
// has no epic-row case to handle.
func (m FlatModel) previewContent(width int) string {
	t, ok := m.selectedTicket()
	if !ok {
		return ui.StyleDim.Render("  no ticket selected")
	}
	status := m.epic.RenderedStatus(t)

	var b strings.Builder
	b.WriteString(previewHeaderLine(m.icons(), status, t))
	if meta := previewMetaLine(m.epic, t, status); meta != "" {
		b.WriteString("\n")
		b.WriteString(meta)
	}
	b.WriteString("\n")
	b.WriteString(previewRuleStyle.Render(strings.Repeat("─", max(width, 0))))
	b.WriteString("\n")
	if t.ReadErr != "" {
		b.WriteString(statusErrorStyle.Render("  error reading ticket file: " + t.ReadErr))
	} else {
		b.WriteString(renderTicketMarkdown(t.Body, width))
	}
	return b.String()
}

func (m FlatModel) previewLines() []string {
	return renderViewportWithScrollbar(m.previewVP)
}
