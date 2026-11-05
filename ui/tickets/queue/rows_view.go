package queue

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ralphloop/schedule"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tree"
)

// RenderOpts builds the tree.RenderOpts m.queueTree.RenderLines renders
// through: every physical row (blank separator, epic status/context/error
// header, ticket) is a real tree.Entry, dispatched on entry.Value.Kind by
// Label alone — mirroring the Tickets-tab sidebar's sidebarRenderOpts one
// level down. Header/separator/reason rows are excluded from selection via
// SetIsSelectable/SkipUnselectable (ticket 17), mirroring the sidebar's own
// nodeEmpty treatment.
func (r Rows) RenderOpts(env RowsEnv, width int) tree.RenderOpts[Node] {
	entries := r.Tree.Entries()
	idxByID := make(map[string]int, len(entries))
	for i, e := range entries {
		idxByID[e.ID] = i
	}
	h := env.Header
	return tree.RenderOpts[Node]{
		AccentColor: ui.ColorBlue,
		Active:      env.SidebarActive,
		Width:       width,
		// EmptyLine is blank: the header banner's idle line (header.go)
		// already covers the empty-selection case with "No selected tickets...".
		EmptyLine: "",
		Label: func(entry tree.Entry[Node]) string {
			switch entry.Value.Kind {
			case NodeEpicSeparator:
				return ""
			case NodeEpicName:
				_, _, fill := h.EpicStatus(entry.Value.Epic)
				return EpicNameLine(entry.Value.Epic.Name, h.Summaries[entry.Value.Epic.Name], fill, entry.Value.Layout, env.LabelWidth)
			case NodeEpicBar:
				_, _, fill := h.EpicStatus(entry.Value.Epic)
				return EpicBarLine(h.Summaries[entry.Value.Epic.Name], fill, entry.Value.Layout)
			case NodeEpicSpacer:
				return ""
			case NodeEpicStatusLine:
				// The cost is dim like the rest of the metrics regardless of the
				// state's color — the configured limits are session-wide, so
				// coloring one epic's slice against them would misleadingly
				// suggest that epic alone is over budget.
				return h.EpicStatusRow(entry.Value.Epic, env.LabelWidth)
			case NodeEpicError:
				if stuck, ok := entry.Value.Err.(*schedule.StuckPlanError); ok {
					return "    " + rowfmt.MetricsLineStyle.Render(
						"no unblocked tickets left: "+strings.Join(stuck.BlockedIDs(), ", ")+
							" still waiting on a dependency cycle or a blocker outside the selection",
					)
				}
				return ui.StyleError.Render("    " + entry.Value.Err.Error())
			default: // NodeTicket
				return r.RenderTicketRow(env, entry.Value.Ticket, idxByID[entry.ID])
			}
		},
	}
}

// RenderTicketRow renders one row's text content for the same
// single-line status presentation as the Tickets tab's renderTicketRow
// (view.go), so the Queue tab shows identical per-ticket status (ticket 25).
// Nesting under a parent ticket (r.Depth>0) is handled by
// Tree.RenderLines itself via each Entry's own Depth — but every
// ticket entry sits at the tree's root alongside the epic header rows (not
// nested under them), so this still carries the same literal "  " base
// indent those header rows carry, keeping ticket and header text aligned at
// depth 0 the way buildQueueLines' old "  "+repeat("  ", r.depth) formula
// did in one combined computation. r.HasChildren/r.Expanded still drive the
// row's own fold triangle since that's row content, not tree indentation —
// the triangle column is reserved at a fixed width whether or not this row
// has children, matching the Tickets tab's renderTicketRow (ticket 09/10).
// Search-match highlighting is likewise left to RenderLines' own generic
// overlay — rowIdx is only used to dim a non-matching row while a query is
// active. A paused (answer)/paused (repair) ticket's pause-reason subtext is a
// second physical line on this same entry (queueTicketReasonLine, set as
// Entry.Body in BuildEntries) rather than rendered here.
func (rs Rows) RenderTicketRow(env RowsEnv, r Row, rowIdx int) string {
	epic, t := r.Epic, r.Ticket
	h := env.Header
	icons := h.Icons
	status := rowfmt.TicketStatus(epic, t)
	indent := " "

	triangle := strings.Repeat(" ", rowfmt.TriangleColumnWidth(icons)) + " "
	if r.HasChildren {
		glyph := icons.TriangleExpanded
		if !r.Expanded {
			glyph = icons.TriangleCollapsed
		}
		triangle = glyph + " "
	}

	if h.RunningEpics[epic.Name] {
		if live, ok := h.Live[epic.Name][t.Identifier]; ok {
			if base, suffix, ok := rowfmt.RenderLiveTicketRow(icons, h.Spinner, t, live, indent+triangle); ok {
				metrics := rowfmt.FormatMetricsLine(rowfmt.LiveElapsedSeconds(live), live.Tokens, 0, env.CodexRows[t.Path])
				return rowfmt.AppendRowMetrics(base, rowfmt.JoinNonEmpty(" ", rowfmt.JoinNonEmpty(" ", suffix, metrics), rowfmt.JoinNonEmpty(" ", env.HarnessLabels[t.Path], env.WaitingLabels[t.Path])), rowfmt.MetricsLineStyle)
			}
		}
	}

	icon, style := rowfmt.StatusIconAndStyle(icons, status)

	matched, _ := rs.Tree.SearchMatch(rowIdx)
	searchDim := env.HasQuery && !matched
	// nonCandidateDim marks a row injected by attachQueueAncestors (ticket
	// 08) to keep a candidate nested under its real parent chain — it reuses
	// StatusDoneStyle's permanent grey rather than ui.StyleDim's transient
	// search-fade, since this row isn't actionable regardless of search state.
	nonCandidateDim := !r.Actionable

	// dimStyle/dimmed resolve searchDim vs nonCandidateDim once, with
	// searchDim taking precedence when both apply, and are then reused for
	// every row element (icon, title, badge, suffix, metrics) so a row can't
	// render a mismatched blend of the two dim styles (ticket 13).
	dimStyle := lipgloss.NewStyle()
	dimmed := false
	switch {
	case searchDim:
		dimStyle, dimmed = ui.StyleDim, true
	case nonCandidateDim:
		dimStyle, dimmed = rowfmt.StatusDoneStyle, true
	}

	title := rowfmt.RowTitle(icons, t)
	if t.ShowsCommitlessSuffix() {
		title += " (commitless)"
	}
	titleStyle := lipgloss.NewStyle()
	switch {
	case status.Terminal():
		titleStyle = rowfmt.StatusDoneStyle
	case dimmed:
		titleStyle = dimStyle
	}
	if dimmed {
		style = dimStyle
	}

	line := indent + triangle + style.Render(icon)
	if env.Suggested(epic, t) {
		badgeStyle := rowfmt.SuggestedActionBadgeStyle
		if dimmed {
			badgeStyle = dimStyle
		}
		line += " " + badgeStyle.Render(icons.SuggestedAction)
	}
	line += " " + titleStyle.Render(title)
	if suffix := rowfmt.BlockedBySuffix(epic, t, status); suffix != "" {
		suffixStyle := rowfmt.BlockedBySuffixStyle
		if dimmed {
			suffixStyle = dimStyle
		}
		line += " " + suffixStyle.Render(suffix)
	}
	metrics := rowfmt.JoinNonEmpty(" ", env.HarnessLabels[t.Path], env.WaitingLabels[t.Path])
	if t.ElapsedTime > 0 || t.ActualContextWindow > 0 || t.ActualCost > 0 {
		metrics = rowfmt.JoinNonEmpty(" ", rowfmt.FormatMetricsLine(t.ElapsedTime, t.ActualContextWindow, t.ActualCost, t.HadCodex), metrics)
	}
	if metrics == "" {
		return line
	}
	if dimmed {
		return rowfmt.AppendRowMetrics(line, metrics, dimStyle)
	}
	return rowfmt.AppendRowMetrics(line, metrics, style)
}

// queueTicketReasonLine renders the pause-reason subtext for a paused (answer)/
// paused (repair) ticket as a second physical line on its own NodeTicket
// entry (set as Entry.Body in BuildEntries), indented to align under the
// title the same way the pre-tree.Model buildQueueLines' two-physical-line
// row used to (triangle + icon + badge width, mirroring RenderTicketRow's
// own column layout). ok reports false (empty line) when r's ticket isn't
// listed in the server's Paused tickets for its epic.
func (rs Rows) queueTicketReasonLine(env RowsEnv, r Row) (line string, ok bool) {
	epic, t := r.Epic, r.Ticket
	icons := env.Header.Icons
	paused := env.Header.Summaries[epic.Name].Paused
	status := rowfmt.TicketStatus(epic, t)
	i := slices.IndexFunc(paused, func(p wire.SummaryTicket) bool { return p.Address == t.Path })
	if i < 0 || paused[i].Reason == "" {
		return "", false
	}
	reason := rowfmt.Ellipsize(paused[i].Reason, rowfmt.PauseReasonMaxRunes, icons.Ellipsis)

	triangleWidth := rowfmt.TriangleColumnWidth(icons) + 1
	icon, _ := rowfmt.StatusIconAndStyle(icons, status)
	badgeWidth := 0
	if env.Suggested(epic, t) {
		badgeWidth = lipgloss.Width(icons.SuggestedAction) + 1
	}

	indent := " "
	prefix := indent + strings.Repeat(" ", triangleWidth+lipgloss.Width(icon)+1+badgeWidth)
	return prefix + rowfmt.BlockedBySuffixStyle.Render(reason), true
}
