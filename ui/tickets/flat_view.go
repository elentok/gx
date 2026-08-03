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
	if m.resumeConfirm.isOpen() {
		content = ui.OverlayCenter(content, m.resumeConfirmView(), m.width, m.height)
	}
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

	var body string
	if m.flatUseStackedLayout() {
		seam := ui.RenderSeamRow(listW, ui.SeamColor)
		body = lipgloss.JoinVertical(lipgloss.Left, listView, seam, previewView)
	} else {
		seam := ui.RenderSeamColumn(h, ui.SeamColor)
		body = lipgloss.JoinHorizontal(lipgloss.Top, listView, seam, previewView)
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, m.footerView())
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
	var lines []string
	for i, t := range m.ordered {
		rows := m.renderFlatTicketRow(t)
		if i == m.selected {
			for j, row := range rows {
				rows[j] = ui.RenderRowHighlight(row)
			}
		}
		lines = append(lines, rows...)
	}
	return lines
}

// renderFlatTicketRow renders t's row, one line for a ticket that's never
// run (open/claimed/blocked/needs-info/error — no elapsed-time/token data to
// show), two lines for a ticket that has: a live running/paused/needs-
// attention entry, or a done ticket's landed metrics. The two-line decision
// is status-based, not data-presence-based — a done ticket whose metrics
// were never stamped (the 0/0 sentinel, e.g. a repair/reattach landing)
// still gets a second line reading "0s · 0 tok" rather than falling back to
// one line.
func (m FlatModel) renderFlatTicketRow(t tickets.Ticket) []string {
	status := m.epic.RenderedStatus(t)

	// A superseded ticket is a terminal disk state that always wins over a
	// live orchestrator entry: reconcile.go deliberately skips superseded
	// tickets when clearing m.live (see reconcile.go:102-105), so a stale
	// paused/needs-attention entry can otherwise outlive the supersession and
	// render this row via the live branch below, which never applies dim
	// styling.
	if status != tickets.StatusSuperseded {
		if live, ok := m.live[t.Identifier]; ok {
			if base, _, ok := renderLiveTicketRow(m.icons(), m.spinner, t, live); ok {
				return []string{base, m.ticketMetricsLine(t, status)}
			}
		}
	}

	icon, style := statusIconAndStyle(m.icons(), status)

	title := fmt.Sprintf("%s %s", t.DisplayNumber(), t.Title)
	titleStyle := lipgloss.NewStyle()
	if status == tickets.StatusDone || status == tickets.StatusSuperseded {
		titleStyle = statusDoneStyle
	}

	line := "  " + style.Render(icon) + " " + titleStyle.Render(title)
	if suffix := blockedBySuffix(m.epic, t, status); suffix != "" {
		line += " " + blockedBySuffixStyle.Render(suffix)
	}

	if status != tickets.StatusDone {
		return []string{line}
	}
	return []string{line, m.ticketMetricsLine(t, status)}
}

// ticketMetricsLine renders t's rendered/styled line-2 metrics text (empty
// for a never-run ticket) — the single source both renderFlatTicketRow's
// list row and previewContent's preview-panel line pull from, per ticket 07,
// so the two always agree.
func (m FlatModel) ticketMetricsLine(t tickets.Ticket, status tickets.RenderedStatus) string {
	if status != tickets.StatusSuperseded {
		if live, ok := m.live[t.Identifier]; ok {
			if _, suffix, ok := renderLiveTicketRow(m.icons(), m.spinner, t, live); ok {
				metrics := formatMetricsLine(liveElapsedSeconds(live), live.tokens)
				return renderMetricsLine(joinNonEmpty(" ", suffix, metrics))
			}
		}
	}
	if status != tickets.StatusDone {
		return ""
	}
	base := formatMetricsLine(t.ElapsedTime, t.ActualContextWindow)
	return renderMetricsLineWithLanded(base, m.icons(), m.landedOK, m.landed[t.Identifier])
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
	if live, ok := m.liveStateForSelected(); ok {
		if meta := previewLiveMetaLine(live); meta != "" {
			b.WriteString("\n")
			b.WriteString(meta)
		}
	}
	if metrics := m.ticketMetricsLine(t, status); metrics != "" {
		b.WriteString("\n")
		b.WriteString(metrics)
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

// previewLiveMetaLine renders live's herdr tab id and pause/attention reason
// as the preview pane's metadata line (ticket 04b) — the row-suffix
// convention of renderLiveTicketRow (flat_live.go), reused here since both
// read from the same liveTicketState.
func previewLiveMetaLine(live liveTicketState) string {
	if live.label == "" && live.reason == "" {
		return ""
	}
	line := "  "
	if live.label != "" {
		line += ui.StyleMuted.Render("tab " + live.label)
	}
	return appendBlockedBySuffix(line, live.reason)
}

func (m FlatModel) previewLines() []string {
	lines := renderViewportWithScrollbar(m.previewVP)
	if _, ok := m.liveStateForSelected(); !ok {
		return lines
	}
	lines = append(lines, previewRuleStyle.Render(strings.Repeat("─", max(m.previewVP.Width(), 0))))
	lines = append(lines, renderViewportWithScrollbar(m.transcriptVP)...)
	return lines
}
