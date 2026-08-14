package tickets

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tree"
)

var (
	// statusOpenStyle deliberately has no Foreground override (ticket 02):
	// open is the default/no-signal state, so it renders in the terminal's
	// own default foreground rather than drawing the eye with a color.
	statusOpenStyle = lipgloss.NewStyle()
	// statusDraftStyle reads as parked, not idle: a draft is outstanding work
	// nobody can pick up, so it sits between open's default foreground and
	// done's near-invisible dim.
	statusDraftStyle       = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	statusClaimedStyle     = lipgloss.NewStyle().Foreground(ui.ColorOrange)
	statusBlockedStyle     = lipgloss.NewStyle().Foreground(ui.ColorRed)
	statusNeedsAnswerStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow)
	statusNeedsRepairStyle = lipgloss.NewStyle().Foreground(ui.ColorRed)
	// statusWaitingForChildrenStyle uses the same color family as claimed —
	// this ticket's own work is finished, but the epic still has live work
	// underneath it, so it reads as active rather than settled/dim like done.
	statusWaitingForChildrenStyle = lipgloss.NewStyle().Foreground(ui.ColorOrange)
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

	// suggestedActionBadgeStyle renders the "m"-menu badge (ticketHasSuggestedActions)
	// distinct from every status color so it reads as an affordance, not a
	// status signal.
	suggestedActionBadgeStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true)

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

// sidebarRenderOpts builds the tree.RenderOpts m.sidebarTree.RenderLines
// renders through: every physical row (section header, empty-section
// placeholder, epic, ticket) is a real tree.Entry, dispatched on
// entry.Value.kind by Label alone (Icon/MetaText/RowColor/Faint stay
// nil — ticket 01's "Row rendering" spec). A section header (ticket 02)
// renders with the same expand-glyph-led shape as an epic/ticket row, minus
// the checkbox slot — no separate Icon/glyph-column plumbing, since epic and
// ticket rows bake their own glyph into the Label string too. --all mode
// behaves the same as single-worktree: it just interleaves every worktree's
// epics into this one grouping, each epic row labeled with its worktree
// (renderEpicRow).
func (m Model) sidebarRenderOpts(width int) tree.RenderOpts[sidebarNode] {
	idxs := make([]int, len(m.epics))
	for i := range m.epics {
		idxs[i] = i
	}
	openIdxs, closedIdxs := splitEpicIndexesBySection(m.epics, idxs)

	entries := m.sidebarTree.Entries()
	idxByID := make(map[string]int, len(entries))
	for i, e := range entries {
		idxByID[e.ID] = i
	}

	return tree.RenderOpts[sidebarNode]{
		AccentColor: ui.ColorBlue,
		Active:      m.focus == focusSidebar,
		Width:       width,
		EmptyLine:   ui.StyleDim.Render("  loading…"),
		Label: func(entry tree.Entry[sidebarNode]) string {
			switch entry.Value.kind {
			case nodeSection:
				glyph := m.icons().TriangleExpanded
				if m.sidebarTree.CollapsedIDs()[entry.ID] {
					glyph = m.icons().TriangleCollapsed
				}
				label, n := "Open epics", len(openIdxs)
				icon := ""
				if entry.Value.section == sectionClosed {
					label, n = "Closed epics", len(closedIdxs)
					icon = m.icons().TicketDone + " "
				}
				return sectionHeaderStyle.Render(fmt.Sprintf("%s %s%s (%d)", glyph, icon, label, n))
			case nodeEmpty:
				label := "open epics"
				if entry.Value.section == sectionClosed {
					label = "closed epics"
				}
				return ui.StyleMuted.Render("no " + label)
			case nodeEpic:
				r, _ := rowFromEntry(entry)
				return m.renderEpicRow(m.epics[r.epicIdx])
			default: // nodeTicket
				r, _ := rowFromEntry(entry)
				return m.renderTicketRow(m.epics[r.epicIdx], r, idxByID[entry.ID])[0]
			}
		},
	}
}

func (m Model) renderEpicRow(epic tickets.Epic) string {
	glyph := m.icons().TriangleExpanded
	if m.isCollapsed(epic) {
		glyph = m.icons().TriangleCollapsed
	}
	line := fmt.Sprintf("%s %s %s (%d done / %d)", glyph, m.checkboxGlyph(m.epicChecked(epic)), epic.Name, epic.DoneCount(), epic.TotalCount())
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

// renderTicketRow renders one physical line for every ticket, unindented —
// ui/tree's renderEntry prepends the entry's own depth-based indent, so a
// nested ticket (Parent/Children, ticket 03) needs no indent handling here.
// The triangle column is reserved at a fixed width whether or not this row
// has children (a childless row shows a blank in its place), so every row
// at a given depth — siblings, and a row's own children one level in —
// lines its checkbox up in the same column; only the triangle itself,
// sitting left of the checkbox, reflects r.expanded. A live or done
// ticket's line ends with the same elapsed/token metrics as the former
// standalone ralph-loop view, appended dim italic; live rows also append
// their phase or pause reason there. Returns a single-element slice today
// (every branch renders exactly one physical line); the []string return
// keeps the multi-line-row seam open for a future ticket without another
// signature change at every call site.
func (m Model) renderTicketRow(epic tickets.Epic, r row, rowIdx int) []string {
	t := epic.Tickets[r.ticketIdx]
	status := epic.RenderedStatus(t)

	triangle := strings.Repeat(" ", triangleColumnWidth(m.icons())) + " "
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
			if base, suffix, ok := renderLiveTicketRow(m.icons(), m.implementSpinner, t, live, triangle+m.checkboxGlyph(m.isChecked(t.Path))+" "); ok {
				metrics := formatMetricsLine(liveElapsedSeconds(live), live.tokens, 0)
				return []string{appendRowMetrics(base, joinNonEmpty(" ", suffix, metrics), metricsLineStyle)}
			}
		}
	}

	icon, style := statusIconAndStyle(m.icons(), status)

	matched, _ := m.searchMatch(rowIdx)
	searchDim := m.search.HasQuery() && !matched
	doneDim := status == tickets.StatusDone

	title := fmt.Sprintf("%s %s", t.DisplayNumber(), t.Title)
	if t.ShowsCommitlessSuffix() {
		title += " (commitless)"
	}
	titleStyle := lipgloss.NewStyle()
	if !matched {
		if doneDim {
			titleStyle = statusDoneStyle
		} else if searchDim {
			titleStyle = ui.StyleDim
		}
	}
	if searchDim {
		style = ui.StyleDim
	}

	line := triangle + m.checkboxGlyph(m.isChecked(t.Path)) + " " + style.Render(icon)
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
		return []string{line}
	}
	metrics := formatMetricsLine(t.ElapsedTime, t.ActualContextWindow, t.ActualCost)
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

// triangleColumnWidth is the fixed column width every row reserves for its
// triangle glyph (ticket 10), so a childless row's blank placeholder lines
// up with a sibling's actual triangle regardless of which collapse-state
// glyph (up or right) is wider.
func triangleColumnWidth(icons ui.IconSet) int {
	w := lipgloss.Width(icons.TriangleExpanded)
	if cw := lipgloss.Width(icons.TriangleCollapsed); cw > w {
		w = cw
	}
	return w
}

// statusIconAndStyle maps a ticket's rendered status to its dedicated glyph
// and color, distinct from the PRs tab's facet icon set.
func statusIconAndStyle(icons ui.IconSet, status tickets.RenderedStatus) (string, lipgloss.Style) {
	switch status {
	case tickets.StatusDraft:
		return icons.TicketDraft, statusDraftStyle
	case tickets.StatusOpen:
		return icons.TicketOpen, statusOpenStyle
	case tickets.StatusClaimed:
		return icons.TicketClaimed, statusClaimedStyle
	case tickets.StatusBlocked:
		return icons.TicketBlocked, statusBlockedStyle
	case tickets.StatusNeedsAnswer:
		return icons.TicketNeedsAnswer, statusNeedsAnswerStyle
	case tickets.StatusNeedsRepair:
		return icons.TicketNeedsRepair, statusNeedsRepairStyle
	case tickets.StatusWaitingForChildren:
		return icons.TicketWaitingForChildren, statusWaitingForChildrenStyle
	case tickets.StatusDone:
		return icons.TicketDone, statusDoneStyle
	default: // tickets.StatusError
		return icons.TicketError, statusErrorStyle
	}
}

// blockedBySuffix renders the "(blocked by NN[, NN...])" suffix for a
// blocked/needs-answer ticket, filtered to still-unresolved blockers. Empty
// for any other status or once every blocker has resolved.
func blockedBySuffix(epic tickets.Epic, t tickets.Ticket, status tickets.RenderedStatus) string {
	if status != tickets.StatusBlocked && status != tickets.StatusNeedsAnswer {
		return ""
	}
	unresolved := epic.UnresolvedBlockers(t)
	if len(unresolved) == 0 {
		return ""
	}
	if len(unresolved) > 3 {
		return fmt.Sprintf("(blocked by %d tickets)", len(unresolved))
	}
	return fmt.Sprintf("(blocked by %s)", strings.Join(unresolved, ", "))
}
