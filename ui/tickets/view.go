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
	// statusDoneStyle is deliberately dimmer than ui.StyleDim/StyleMuted
	// (used elsewhere for transient states like search-fade or loading
	// text): "done" is a permanent, low-priority state that should read as
	// clearly less prominent than everything else in the row, not merely
	// muted.
	statusDoneStyle  = lipgloss.NewStyle().Foreground(ui.ColorOverlay).Faint(true)
	statusErrorStyle = lipgloss.NewStyle().Foreground(ui.ColorRed).Bold(true)

	blockedBySuffixStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle).Italic(true)

	sectionHeaderStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle)

	// worktreeTagStyle renders --all mode's per-epic worktree label, appended
	// after the epic's name/count.
	worktreeTagStyle = lipgloss.NewStyle().Foreground(ui.ColorBlue)
)

// sidebarLines renders the epic/ticket tree as exactly two headed sections —
// "Open epics" then "Closed epics" (mirroring the PRs tab's
// Actionable/Non-actionable split), the same in --all mode as in the
// single-worktree view: --all just interleaves every worktree's epics into
// this one grouping, each epic row labeled with its worktree (renderEpicRow).
// Each epic's expand glyph + name + (open/total) count, each ticket's status
// icon + title indented beneath it, grouped and collapsed per visibleRows.
// Row highlighting/search indexing uses each row's position in
// visibleRows() (i).
func (m Model) sidebarLines() []string {
	if !m.loaded {
		return []string{ui.StyleDim.Render("  loading…")}
	}
	if len(m.epics) == 0 {
		return []string{ui.StyleMuted.Render("  no .scratch/ directory found")}
	}

	idxs := make([]int, len(m.epics))
	for i := range m.epics {
		idxs[i] = i
	}
	openIdxs, closedIdxs := splitEpicIndexesBySection(m.epics, idxs)

	var lines []string
	i := 0 // running position within the full visibleRows() slice

	lines = append(lines, sectionHeaderStyle.Render(fmt.Sprintf("── Open epics (%d) ──", len(openIdxs))))
	if len(openIdxs) == 0 {
		lines = append(lines, ui.StyleMuted.Render("  no open epics"))
	}
	openRows := m.rowsForEpicOrder(openIdxs)
	lines = m.appendRowLines(lines, openRows, i)
	i += len(openRows)

	lines = append(lines, "", sectionHeaderStyle.Render(fmt.Sprintf("── Closed epics (%d) ──", len(closedIdxs))))
	if len(closedIdxs) == 0 {
		lines = append(lines, ui.StyleMuted.Render("  no closed epics"))
	}
	closedRows := m.rowsForEpicOrder(closedIdxs)
	lines = m.appendRowLines(lines, closedRows, i)
	i += len(closedRows)

	return lines
}

// appendRowLines renders rows (a contiguous slice of visibleRows()) onto
// lines, where startIdx is rows[0]'s position in the full visibleRows()
// slice — needed so selection highlighting and search-match indexing (both
// keyed by row position) stay correct despite the interleaved section
// headers.
func (m Model) appendRowLines(lines []string, rows []row, startIdx int) []string {
	for offset, r := range rows {
		i := startIdx + offset
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
	line := fmt.Sprintf("  %s %s (%d done / %d)", glyph, epic.Name, epic.DoneCount(), epic.TotalCount())
	// Dimming tracks "every ticket done", not the current collapse toggle —
	// a fully-done epic stays dimmed even if manually expanded, and a
	// manually-collapsed in-progress epic doesn't borrow its dimming.
	if epic.AllDone() {
		line = statusDoneStyle.Render(line)
	}
	if epic.WorktreeName != "" {
		line += " " + worktreeTagStyle.Render("["+epic.WorktreeName+"]")
	}
	return line
}

func (m Model) renderTicketRow(epic tickets.Epic, t tickets.Ticket, rowIdx int) string {
	status := epic.RenderedStatus(t)
	icon, style := statusIconAndStyle(m.icons(), status)

	matched, current := m.searchMatch(rowIdx)
	searchDim := m.search.HasQuery() && !matched
	doneDim := status == tickets.StatusDone

	title := fmt.Sprintf("%d %s", t.Number, t.Title)
	titleStyle := lipgloss.NewStyle()
	if matched {
		title = search.Highlight(title, m.search.Query(), current)
	} else if doneDim {
		titleStyle = statusDoneStyle
	} else if searchDim {
		titleStyle = ui.StyleDim
	}
	if searchDim {
		style = ui.StyleDim
	}

	line := "    " + style.Render(icon) + " " + titleStyle.Render(title)
	if suffix := blockedBySuffix(epic, t, status); suffix != "" {
		suffixStyle := blockedBySuffixStyle
		if searchDim {
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
