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

func (m QueueModel) View() tea.View {
	if m.queueStore != nil {
		snapshot := m.queueStore.Snapshot()
		m.checked = snapshot.Checked
		m.checkOrder = snapshot.Order
		m.queueStatus = snapshot.Status
	}
	if !m.ready {
		return ui.NewMainView("\n  Initializing…")
	}
	viewportH := m.queueViewportHeight()
	height := max(m.height-1, 1)
	sidebarW, previewW := splitPanelWidth(m.width)
	sidebarH, previewH := splitPanelHeight(m.width, height)
	lines := ui.AppendScrollbar(m.queueVisibleLines(viewportH), sidebarW-2, len(m.queueLines()), viewportH, m.scrollOffset)

	queueView := ui.RenderPanel(ui.PanelOptionsFor(
		sidebarW, sidebarH, m.queueHeaderTitle(), "", lines, true, ui.ColorBlue, nil, true,
	))
	previewView := ui.RenderPanel(ui.PanelOptionsFor(
		previewW, previewH, "Preview", m.previewMatchStatus(), m.previewLines(), false, ui.ColorBlue, nil, false,
	))

	var content string
	if useStackedLayout(m.width) {
		seam := ui.RenderSeamRow(sidebarW, ui.SeamColor)
		content = lipgloss.JoinVertical(lipgloss.Left, queueView, seam, previewView)
	} else {
		seam := ui.RenderSeamColumn(sidebarH, ui.SeamColor)
		content = lipgloss.JoinHorizontal(lipgloss.Top, queueView, seam, previewView)
	}

	if m.implementAgentMenuOpen {
		plans := m.checkedEpicPlans()
		menu := renderImplementAgentMenu(
			fmt.Sprintf("Choose the agent for %d checked epic(s):", len(plans)),
			m.implementAgentMenu,
		)
		content = ui.OverlayCenter(content, menu, m.width, m.height)
	} else if m.confirm.IsOpen {
		content = ui.OverlayCenter(content, m.confirm.View(m.width), m.width, m.height)
	}
	if m.search.Mode() == search.SearchModeInput {
		overlayW := m.searchOverlayWidth()
		activeSearch := m.search
		activeSearch.SetWidth(overlayW)
		overlay := activeSearch.View()
		y := m.settings.InputModalBottom.ResolveY(m.height, lipgloss.Height(overlay))
		content = ui.OverlayBottomCenter(content, overlay, m.width, y)
	}
	return ui.NewMainView(content)
}

// queueVisibleLines windows queueLines() to a single viewportH-line scroll
// position at m.scrollOffset, mirroring Model.sidebarVisibleLines.
func (m QueueModel) queueVisibleLines(viewportH int) []string {
	lines := m.queueLines()
	start := min(m.scrollOffset, len(lines))
	end := min(start+viewportH, len(lines))
	return lines[start:end]
}

func (m QueueModel) completedExecutionProgress() (done, total int) {
	for _, epic := range m.epics {
		for _, ticket := range epic.Tickets {
			if !m.executionTickets[epic.Name+"/"+ticket.Identifier] {
				continue
			}
			total++
			if epic.RenderedStatus(ticket) == tickets.StatusDone {
				done++
			}
		}
	}
	return done, total
}

// checkedProgress reports the active run's done/total ticket counts, scoped
// to m.executionTickets — the run's captured selection at kickoff — rather
// than the live m.checked set, so editing the checked selection while a run
// is active doesn't rewrite that run's progress totals (ticket 20).
func (m QueueModel) checkedProgress() (int, int) {
	done := 0
	total := 0
	for _, epic := range m.epics {
		for _, ticket := range epic.Tickets {
			if !m.executionTickets[epic.Name+"/"+ticket.Identifier] {
				continue
			}
			total++
			if epic.RenderedStatus(ticket) == tickets.StatusDone || m.queueStatus[ticket.Path] == queueStatusDone {
				done++
			}
		}
	}
	return done, total
}

func (m QueueModel) queueLines() []string {
	lines, _, _ := m.buildQueueLines()
	return lines
}

// queueLineForSelected returns the selected row's line index and physical
// height within queueLines()'s output, mirroring Model.sidebarLineForSelected
// — needed since a row is one or two physical lines depending on its
// live/done status (renderQueueTicketRow).
func (m QueueModel) queueLineForSelected() (line, height int, ok bool) {
	_, offsets, heights := m.buildQueueLines()
	if m.selected < 0 || m.selected >= len(offsets) {
		return 0, 0, false
	}
	return offsets[m.selected], heights[m.selected], true
}

// buildQueueLines renders every candidate ticket, grouped by epic and
// ordered in plan order (rowsAndPlanErrors), as the same single-line status
// rows the Tickets tab renders for its own tickets (renderQueueTicketRow) — no
// "parallel"/"then" wave grouping (ticket 25). offsets/heights are aligned to
// rowsAndPlanErrors' row order so selection/scroll math can find any row's
// position in lines without re-deriving the rendering.
func (m QueueModel) buildQueueLines() (lines []string, offsets []int, heights []int) {
	if !m.loaded {
		return []string{ui.StyleDim.Render("  loading…")}, nil, nil
	}

	for _, line := range m.queueHeaderBodyLines() {
		lines = append(lines, "  "+ui.StyleHint.Render(line))
	}

	rows, planErrs := m.rowsAndPlanErrors()
	if len(rows) == 0 {
		lines = append(lines, ui.StyleMuted.Render("  no tickets checked — check tickets in the Tickets tab to build a plan"))
		return lines, nil, nil
	}

	epicName := ""
	for i, r := range rows {
		if r.epic.Name != epicName {
			epicName = r.epic.Name
			lines = append(lines, "")
			lines = append(lines, m.epicHeaderLines(r.epic)...)
			if err, ok := planErrs[epicName]; ok {
				lines = append(lines, statusErrorStyle.Render("    "+err.Error()))
			}
		}
		offsets = append(offsets, len(lines))
		rowLines := m.renderQueueTicketRow(r, i)
		if i == m.selected {
			for li, line := range rowLines {
				rowLines[li] = ui.RenderRowHighlight(line)
			}
		}
		lines = append(lines, rowLines...)
		heights = append(heights, len(rowLines))
	}
	return lines, offsets, heights
}

// renderQueueTicketRow renders one physical line for every ticket — the same
// single-line status presentation as the Tickets tab's renderTicketRow
// (view.go), so the Queue tab shows identical per-ticket status (ticket 25).
// r.depth indents a nested ticket (Parent/Children, ticket 03) two extra
// spaces per level, matching ui/tree's own indent unit and the Tickets tab's
// renderTicketRow (ticket 09); a ticket with children gets the same
// folder-open/closed glyph an epic row uses in the Tickets tab, reflecting
// r.expanded.
func (m QueueModel) renderQueueTicketRow(r queueRow, rowIdx int) []string {
	epic, t := r.epic, r.ticket
	status := epic.RenderedStatus(t)
	indent := "  " + strings.Repeat("  ", r.depth)

	if m.runningEpics[epic.Name] {
		if live, ok := m.live[epic.Name][t.Identifier]; ok {
			if base, suffix, ok := renderLiveTicketRow(m.icons(), m.implementSpinner, t, live, indent); ok {
				metrics := formatMetricsLine(liveElapsedSeconds(live), live.tokens)
				return []string{appendRowMetrics(base, joinNonEmpty(" ", suffix, metrics), metricsLineStyle)}
			}
		}
	}

	icon, style := statusIconAndStyle(m.icons(), status)

	matched, current := m.queueSearchMatch(rowIdx)
	searchDim := m.search.HasQuery() && !matched

	title := fmt.Sprintf("%s %s", t.DisplayNumber(), t.Title)
	if t.Commitless {
		title += " (commitless)"
	}
	titleStyle := lipgloss.NewStyle()
	if matched {
		title = search.Highlight(title, m.search.Query(), current)
	} else if status == tickets.StatusDone {
		titleStyle = statusDoneStyle
	} else if searchDim {
		titleStyle = ui.StyleDim
	}
	if searchDim {
		style = ui.StyleDim
	}

	fold := ""
	if r.hasChildren {
		glyph := m.icons().FolderOpen
		if !r.expanded {
			glyph = m.icons().FolderClosed
		}
		fold = glyph + " "
	}

	line := indent + fold + style.Render(icon) + " " + titleStyle.Render(title)
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
	return []string{appendRowMetrics(line, metrics, style)}
}

func (m QueueModel) icons() ui.IconSet {
	return ui.Icons(m.settings.UseNerdFontIcons)
}
