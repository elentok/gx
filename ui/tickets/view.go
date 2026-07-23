package tickets

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"
)

var (
	statusOpenStyle      = lipgloss.NewStyle().Foreground(ui.ColorGreen)
	statusClaimedStyle   = lipgloss.NewStyle().Foreground(ui.ColorBlue)
	statusBlockedStyle   = lipgloss.NewStyle().Foreground(ui.ColorRed)
	statusNeedsInfoStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow)
	statusDoneStyle      = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	statusErrorStyle     = lipgloss.NewStyle().Foreground(ui.ColorRed).Bold(true)

	blockedBySuffixStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle).Italic(true)
)

// sidebarLines renders the flat epic/ticket tree: each epic's expand glyph +
// name + (open/total) count, each ticket's status icon + title indented
// beneath it, grouped and collapsed per visibleRows.
func (m Model) sidebarLines() []string {
	if !m.loaded {
		return []string{ui.StyleDim.Render("  loading…")}
	}
	if len(m.epics) == 0 {
		return []string{ui.StyleMuted.Render("  no .scratch/ directory found")}
	}

	rows := m.visibleRows()
	lines := make([]string, 0, len(rows))
	for i, r := range rows {
		selected := i == m.selected
		var line string
		if r.isEpic() {
			line = m.renderEpicRow(m.epics[r.epicIdx])
		} else {
			epic := m.epics[r.epicIdx]
			line = m.renderTicketRow(epic, epic.Tickets[r.ticketIdx], i)
		}
		if selected {
			line = ui.RenderRowHighlight(line)
		}
		lines = append(lines, line)
	}
	return lines
}

func (m Model) renderEpicRow(epic tickets.Epic) string {
	glyph := m.icons().FolderOpen
	if m.isCollapsed(epic) {
		glyph = m.icons().FolderClosed
	}
	line := fmt.Sprintf("  %s %s (%d/%d)", glyph, epic.Name, epic.OpenCount(), epic.TotalCount())
	// Dimming tracks "every ticket done", not the current collapse toggle —
	// a fully-done epic stays dimmed even if manually expanded, and a
	// manually-collapsed in-progress epic doesn't borrow its dimming.
	if epic.AllDone() {
		line = ui.StyleDim.Render(line)
	}
	return line
}

func (m Model) renderTicketRow(epic tickets.Epic, t tickets.Ticket, rowIdx int) string {
	status := epic.RenderedStatus(t)
	icon, style := statusIconAndStyle(m.icons(), status)

	matched, current := m.searchMatch(rowIdx)
	dim := m.search.HasQuery() && !matched

	title := t.Title
	titleStyle := lipgloss.NewStyle()
	if matched {
		title = search.Highlight(title, m.search.Query(), current)
	} else if dim {
		titleStyle = ui.StyleDim
	}
	if dim {
		style = ui.StyleDim
	}

	line := "    " + style.Render(icon) + " " + titleStyle.Render(title)
	if suffix := blockedBySuffix(epic, t, status); suffix != "" {
		suffixStyle := blockedBySuffixStyle
		if dim {
			suffixStyle = ui.StyleDim
		}
		line += " " + suffixStyle.Render(suffix)
	}
	return line
}

func (m Model) icons() ui.IconSet {
	return ui.Icons(m.settings.UseNerdFontIcons)
}

// statusIconAndStyle maps a ticket's rendered status to its dedicated glyph
// and color, distinct from the PRs tab's facet icon set.
func statusIconAndStyle(icons ui.IconSet, status tickets.RenderedStatus) (string, lipgloss.Style) {
	switch status {
	case tickets.StatusOpen:
		return icons.TicketOpen, statusOpenStyle
	case tickets.StatusClaimed:
		return icons.TicketClaimed, statusClaimedStyle
	case tickets.StatusBlocked:
		return icons.TicketBlocked, statusBlockedStyle
	case tickets.StatusNeedsInfo:
		return icons.TicketNeedsInfo, statusNeedsInfoStyle
	case tickets.StatusDone:
		return icons.TicketDone, statusDoneStyle
	default: // tickets.StatusError
		return icons.TicketError, statusErrorStyle
	}
}

// blockedBySuffix renders the "(blocked by NN[, NN...])" suffix for a
// blocked/needs-info ticket, filtered to still-unresolved blockers. Empty
// for any other status or once every blocker has resolved.
func blockedBySuffix(epic tickets.Epic, t tickets.Ticket, status tickets.RenderedStatus) string {
	if status != tickets.StatusBlocked && status != tickets.StatusNeedsInfo {
		return ""
	}
	unresolved := epic.UnresolvedBlockers(t)
	if len(unresolved) == 0 {
		return ""
	}
	numbers := make([]string, len(unresolved))
	for i, n := range unresolved {
		numbers[i] = strconv.Itoa(n)
	}
	return fmt.Sprintf("(blocked by %s)", strings.Join(numbers, ", "))
}
