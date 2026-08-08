package tickets

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"
)

var (
	// statusOpenStyle deliberately has no Foreground override (ticket 02):
	// open is the default/no-signal state, so it renders in the terminal's
	// own default foreground rather than drawing the eye with a color.
	statusOpenStyle           = lipgloss.NewStyle()
	statusClaimedStyle        = lipgloss.NewStyle().Foreground(ui.ColorOrange)
	statusBlockedStyle        = lipgloss.NewStyle().Foreground(ui.ColorRed)
	statusNeedsInfoStyle      = lipgloss.NewStyle().Foreground(ui.ColorYellow)
	statusNeedsAttentionStyle = lipgloss.NewStyle().Foreground(ui.ColorRed)
	// statusDoneStyle is deliberately dimmer than ui.StyleDim/StyleMuted
	// (used elsewhere for transient states like search-fade or loading
	// text): "done" is a permanent, low-priority state that should read as
	// clearly less prominent than everything else in the row, not merely
	// muted.
	statusDoneStyle  = lipgloss.NewStyle().Foreground(ui.ColorOverlay).Faint(true)
	statusErrorStyle = lipgloss.NewStyle().Foreground(ui.ColorRed).Bold(true)

	// statusPausedStyle renders a live orchestrator pause (rate-limit/
	// smart-zone — see ralph-loop's FlatModel), distinct from every disk-only
	// status color above.
	statusPausedStyle = lipgloss.NewStyle().Foreground(ui.ColorMauve)

	blockedBySuffixStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtleLight).Italic(true)

	sectionHeaderStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle)

	// checkedGlyphStyle/uncheckedGlyphStyle render the execution queue's
	// checkbox marker (ticket 04), distinct from every status color so a
	// checked row reads the same regardless of its ticket status.
	// uncheckedGlyphStyle deliberately isn't ui.StyleDim/StyleMuted — those are
	// reserved for the search-non-match/done-row dim treatment elsewhere in
	// this file, which must stay independent of a row's checked state.
	checkedGlyphStyle   = lipgloss.NewStyle().Foreground(ui.ColorGreen)
	uncheckedGlyphStyle = lipgloss.NewStyle().Foreground(ui.ColorOverlay)
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
		var rowLines []string
		if r.isEpic() {
			rowLines = []string{m.renderEpicRow(m.epics[r.epicIdx])}
		} else {
			rowLines = m.renderTicketRow(m.epics[r.epicIdx], r, i)
		}
		for i, line := range rowLines {
			if selected {
				rowLines[i] = ui.RenderRowHighlight(line)
			}
		}
		lines = append(lines, rowLines...)
	}
	return lines
}

func (m Model) renderEpicRow(epic tickets.Epic) string {
	glyph := m.icons().TriangleExpanded
	if m.isCollapsed(epic) {
		glyph = m.icons().TriangleCollapsed
	}
	line := fmt.Sprintf("  %s %s %s (%d done / %d)", glyph, m.checkboxGlyph(m.epicChecked(epic)), epic.Name, epic.DoneCount(), epic.TotalCount())
	// Dimming tracks "every ticket done", not the current collapse toggle —
	// a fully-done epic stays dimmed even if manually expanded, and a
	// manually-collapsed in-progress epic doesn't borrow its dimming.
	if epic.AllDone() {
		line = statusDoneStyle.Render(line)
	}
	if dur, ok := epic.CompletionDuration(); ok {
		line += " took " + formatDuration(dur)
	}
	if m.implementingEpics[epic.Name] {
		line += " " + statusClaimedStyle.Render(strings.TrimRight(m.implementSpinner.View(), " ")+" running")
	}
	return line
}

// renderTicketRow renders one physical line for every ticket. A live or done
// ticket's line ends with the same elapsed/token metrics as the former
// standalone ralph-loop view, appended dim italic; live rows also append
// their phase or pause reason there. r.depth indents a nested ticket
// (Parent/Children, ticket 03) two extra spaces per level beneath the base
// indent, matching ui/tree's own indent unit. A ticket with children gets a
// small triangle to the left of its checkbox, reflecting r.expanded; a
// childless row has no triangle and no reserved space in its place.
func (m Model) renderTicketRow(epic tickets.Epic, r row, rowIdx int) []string {
	t := epic.Tickets[r.ticketIdx]
	status := epic.RenderedStatus(t)
	indent := "    " + strings.Repeat("  ", r.depth)

	triangle := ""
	if r.hasChildren {
		glyph := m.icons().TriangleExpanded
		if !r.expanded {
			glyph = m.icons().TriangleCollapsed
		}
		triangle = glyph + " "
	}

	// m.live is nested by epic name (ticket 05) precisely because bare
	// ticket identifiers repeat across epics (each restarts numbering from
	// 01) — gating on m.implementingEpics[epic.Name] and looking the ticket
	// up within that epic's own inner map keeps a concurrently-running
	// epic's same-numbered ticket (e.g. two epics' own "02") from
	// cross-rendering as running here.
	if m.implementingEpics[epic.Name] {
		if live, ok := m.live[epic.Name][t.Identifier]; ok {
			if base, suffix, ok := renderLiveTicketRow(m.icons(), m.implementSpinner, t, live, indent+triangle+m.checkboxGlyph(m.isChecked(t.Path))+" "); ok {
				metrics := formatMetricsLine(liveElapsedSeconds(live), live.tokens)
				return []string{appendRowMetrics(base, joinNonEmpty(" ", suffix, metrics), metricsLineStyle)}
			}
		}
	}

	icon, style := statusIconAndStyle(m.icons(), status)

	matched, current := m.searchMatch(rowIdx)
	searchDim := m.search.HasQuery() && !matched
	doneDim := status == tickets.StatusDone

	title := fmt.Sprintf("%s %s", t.DisplayNumber(), t.Title)
	if t.Commitless {
		title += " (commitless)"
	}
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

	line := indent + triangle + m.checkboxGlyph(m.isChecked(t.Path)) + " " + style.Render(icon) + " " + titleStyle.Render(title)
	if suffix := blockedBySuffix(epic, t, status); suffix != "" {
		suffixStyle := blockedBySuffixStyle
		if searchDim {
			suffixStyle = ui.StyleDim
		}
		line += " " + suffixStyle.Render(suffix)
	}
	if status != tickets.StatusDone {
		return []string{line}
	}
	metrics := formatMetricsLine(t.ElapsedTime, t.ActualContextWindow)
	if searchDim {
		return []string{appendRowMetrics(line, metrics, ui.StyleDim)}
	}
	return []string{appendRowMetrics(line, metrics, style)}
}

// checkboxGlyph renders the execution-queue checkbox marker for a checked or
// unchecked row (ticket 04's visible selection state).
func (m Model) checkboxGlyph(checked bool) string {
	if checked {
		return checkedGlyphStyle.Render(m.icons().CheckboxChecked)
	}
	return uncheckedGlyphStyle.Render(m.icons().CheckboxUnchecked)
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
	case tickets.StatusNeedsAttention:
		return icons.TicketNeedsAttention, statusNeedsAttentionStyle
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
	return fmt.Sprintf("(blocked by %s)", strings.Join(unresolved, ", "))
}
