package tickets

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tree"
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
	if m.loaded {
		// m.checked is a map shared with the Tickets tab (and, with a
		// queueStore, refreshed from its snapshot just above) — either can add
		// a ticket to it between Update calls with no queueEpicsLoadedMsg/
		// clampSelected in between, so the tree must be rebuilt on every render
		// rather than only on the events clampSelected's own doc comment lists.
		m.queueTree.SetEntries(m.buildQueueEntries())
	}
	sidebarW, previewW := splitPanelWidth(m.width)
	sidebarH, previewH := splitPanelHeight(m.width, m.contentHeight())

	queueView := ui.RenderPanel(ui.PanelOptionsFor(
		sidebarW, sidebarH, m.queueHeaderTitle(), "", m.queueBody(sidebarW-2), m.focus == focusSidebar, ui.ColorBlue, nil, true,
	))
	previewView := ui.RenderPanel(ui.PanelOptionsFor(
		previewW, previewH, "Preview", m.previewMatchStatus(), m.previewLines(), m.focus == focusPreview, ui.ColorBlue, nil, false,
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
	} else if m.actionsMenu.IsOpen {
		content = ui.OverlayCenter(content, m.actionsMenu.View(), m.width, m.height)
	} else if m.confirm.IsOpen {
		content = ui.OverlayCenter(content, m.confirm.View(m.width), m.width, m.height)
	}
	if m.help.IsOpen {
		content = ui.OverlayCenter(content, m.help.View(), m.width, m.height)
	}
	if m.search.Mode() == search.SearchModeInput {
		overlayW := m.searchOverlayWidth()
		activeSearch := m.search
		activeSearch.SetWidth(overlayW)
		overlay := activeSearch.View()
		y := m.settings.InputModalBottom.ResolveY(m.height, lipgloss.Height(overlay))
		content = ui.OverlayBottomCenter(content, overlay, m.width, y)
	}
	if prefix := m.keys.Prefix(); len(prefix) > 0 {
		hints := ui.ChordBindingsFromHints(m.keys.ChordHints())
		if len(hints) > 0 {
			content = ui.OverlayBottomRight(content, ui.RenderChordOverlay(prefix[0], hints), m.width, m.height)
		}
	}
	return ui.NewMainView(content)
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

// queueBody renders the Queue tab's panel body: the run-state banner
// (queueHeaderBodyLines, queue_header.go) as fixed lines, followed by
// m.queueTree's own windowed rendering — mirroring the Tickets tab's
// sidebarBody split between its own pre-load short-circuit and
// sidebarTree.RenderLines.
func (m QueueModel) queueBody(width int) []string {
	if !m.loaded {
		return []string{ui.StyleDim.Render("  loading…")}
	}

	var lines []string
	for _, line := range m.queueHeaderBodyLines() {
		if strings.Contains(line, "\x1b") {
			// Already carries its own ANSI styling (e.g. a rendered key hint
			// via ui.StatusWithHints) — wrapping it in StyleHint again would
			// bleed one color's reset codes into the other's spans.
			lines = append(lines, "  "+line)
			continue
		}
		lines = append(lines, "  "+ui.StyleHint.Render(line))
	}

	treeH := m.queueViewportHeight() - len(lines)
	lines = append(lines, m.queueTree.RenderLines(treeH+2, m.queueRenderOpts(width))...)
	return lines
}

// queueRenderOpts builds the tree.RenderOpts m.queueTree.RenderLines renders
// through: every physical row (blank separator, epic status/context/error
// header, ticket) is a real tree.Entry, dispatched on entry.Value.kind by
// Label alone — mirroring the Tickets-tab sidebar's sidebarRenderOpts one
// level down. Header/separator/reason rows are excluded from selection via
// SetIsSelectable/SkipUnselectable (ticket 17), mirroring the sidebar's own
// nodeBlank/nodeEmpty treatment.
func (m QueueModel) queueRenderOpts(width int) tree.RenderOpts[queueNode] {
	entries := m.queueTree.Entries()
	idxByID := make(map[string]int, len(entries))
	for i, e := range entries {
		idxByID[e.ID] = i
	}

	return tree.RenderOpts[queueNode]{
		AccentColor: ui.ColorBlue,
		Active:      m.focus == focusSidebar,
		Width:       width,
		EmptyLine:   ui.StyleMuted.Render("  no tickets checked — check tickets in the Tickets tab to build a plan"),
		Label: func(entry tree.Entry[queueNode]) string {
			switch entry.Value.kind {
			case nodeEpicSeparator:
				return ""
			case nodeEpicStatus:
				parkedStalled, _ := ralphLoopRegistry.parkedStalledFor(entry.Value.epic.Name)
				icon, text, style := epicStatusLine(m.icons(), entry.Value.epic, parkedStalled)
				return " " + epicHeaderStyle.Render(entry.Value.epic.Name) + " " + style.Render(icon+" "+text)
			case nodeEpicContext:
				avg, maximum, compacts := epicContextMetrics(entry.Value.epic)
				return " " + metricsLineStyle.Render(fmt.Sprintf(
					"Context window: avg %s, max %s (%d compacts)",
					formatTokenCount(avg), formatTokenCount(maximum), compacts,
				))
			case nodeEpicError:
				return statusErrorStyle.Render("    " + entry.Value.err.Error())
			default: // nodeQueueTicket
				return m.renderQueueTicketRow(entry.Value.ticket, idxByID[entry.ID])
			}
		},
	}
}

// renderQueueTicketRow renders one row's text content for the same
// single-line status presentation as the Tickets tab's renderTicketRow
// (view.go), so the Queue tab shows identical per-ticket status (ticket 25).
// Nesting under a parent ticket (r.depth>0) is handled by
// m.queueTree.RenderLines itself via each Entry's own Depth — but every
// ticket entry sits at the tree's root alongside the epic header rows (not
// nested under them), so this still carries the same literal "  " base
// indent those header rows carry, keeping ticket and header text aligned at
// depth 0 the way buildQueueLines' old "  "+repeat("  ", r.depth) formula
// did in one combined computation. r.hasChildren/r.expanded still drive the
// row's own fold triangle since that's row content, not tree indentation —
// the triangle column is reserved at a fixed width whether or not this row
// has children, matching the Tickets tab's renderTicketRow (ticket 09/10).
// Search-match highlighting is likewise left to RenderLines' own generic
// overlay — rowIdx is only used to dim a non-matching row while a query is
// active. A needs-answer/needs-repair ticket's park-reason subtext is a
// second physical line on this same entry (queueTicketReasonLine, set as
// Entry.Body in buildQueueEntries) rather than rendered here.
func (m QueueModel) renderQueueTicketRow(r queueRow, rowIdx int) string {
	epic, t := r.epic, r.ticket
	status := epic.RenderedStatus(t)
	indent := " "

	triangle := strings.Repeat(" ", triangleColumnWidth(m.icons())) + " "
	if r.hasChildren {
		glyph := m.icons().TriangleExpanded
		if !r.expanded {
			glyph = m.icons().TriangleCollapsed
		}
		triangle = glyph + " "
	}

	if m.runningEpics[epic.Name] {
		if live, ok := m.live[epic.Name][t.Identifier]; ok {
			if base, suffix, ok := renderLiveTicketRow(m.icons(), m.implementSpinner, t, live, indent+triangle); ok {
				metrics := formatMetricsLine(liveElapsedSeconds(live), live.tokens, 0)
				return appendRowMetrics(base, joinNonEmpty(" ", suffix, metrics), metricsLineStyle)
			}
		}
	}

	icon, style := statusIconAndStyle(m.icons(), status)

	matched, _ := m.queueTree.SearchMatch(rowIdx)
	searchDim := m.search.HasQuery() && !matched

	title := fmt.Sprintf("%s %s", t.DisplayNumber(), t.Title)
	if t.ShowsCommitlessSuffix() {
		title += " (commitless)"
	}
	titleStyle := lipgloss.NewStyle()
	if status == tickets.StatusDone {
		titleStyle = statusDoneStyle
	} else if searchDim {
		titleStyle = ui.StyleDim
	}
	if searchDim {
		style = ui.StyleDim
	}

	line := indent + triangle + style.Render(icon)
	if ticketHasSuggestedActions(status, t) {
		badgeStyle := suggestedActionBadgeStyle
		if searchDim {
			badgeStyle = ui.StyleDim
		}
		line += " " + badgeStyle.Render(m.icons().SuggestedAction)
	}
	line += " " + titleStyle.Render(title)
	if suffix := blockedBySuffix(epic, t, status); suffix != "" {
		suffixStyle := blockedBySuffixStyle
		if searchDim {
			suffixStyle = ui.StyleDim
		}
		line += " " + suffixStyle.Render(suffix)
	}
	if status != tickets.StatusDone && t.ElapsedTime <= 0 && t.ActualContextWindow <= 0 {
		return line
	}
	metrics := formatMetricsLine(t.ElapsedTime, t.ActualContextWindow, t.ActualCost)
	if searchDim {
		return appendRowMetrics(line, metrics, ui.StyleDim)
	}
	return appendRowMetrics(line, metrics, style)
}

// queueTicketReasonLine renders the park-reason subtext for a needs-answer/
// needs-repair ticket as a second physical line on its own nodeQueueTicket
// entry (set as Entry.Body in buildQueueEntries), indented to align under the
// title the same way the pre-tree.Model buildQueueLines' two-physical-line
// row used to (triangle + icon + badge width, mirroring renderQueueTicketRow's
// own column layout). ok reports false (empty line) when r's ticket isn't
// currently parked.
func (m QueueModel) queueTicketReasonLine(r queueRow) (line string, ok bool) {
	epic, t := r.epic, r.ticket
	status := epic.RenderedStatus(t)
	reason := parkReason(epic, t, m.icons().Ellipsis)
	if reason == "" {
		return "", false
	}

	triangleWidth := triangleColumnWidth(m.icons()) + 1
	icon, _ := statusIconAndStyle(m.icons(), status)
	badgeWidth := 0
	if ticketHasSuggestedActions(status, t) {
		badgeWidth = lipgloss.Width(m.icons().SuggestedAction) + 1
	}

	indent := " "
	prefix := indent + strings.Repeat(" ", triangleWidth+lipgloss.Width(icon)+1+badgeWidth)
	return prefix + blockedBySuffixStyle.Render(reason), true
}

func (m QueueModel) icons() ui.IconSet {
	return ui.Icons(m.settings.UseNerdFontIcons)
}
