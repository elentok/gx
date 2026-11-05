package tickets

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/server/wire"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tickets/preview"
	"github.com/elentok/gx/ui/tickets/queue"
	"github.com/elentok/gx/ui/tickets/rowfmt"
	"github.com/elentok/gx/ui/tree"
)

var (
	sessionMarkerStyle = lipgloss.NewStyle().Foreground(ui.ColorTeal)

	sectionHeaderStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
)

// sidebarRenderOpts builds the tree.RenderOpts m.sidebarTree.RenderLines
// renders through: every physical row (section header, empty-section
// placeholder, epic, ticket) is a real tree.Entry, dispatched on
// entry.Value.kind by Label alone (Icon/MetaText/RowColor/Faint stay
// nil — ticket 01's "Row rendering" spec). A section header (ticket 02)
// renders with the same expand-glyph-led shape as an epic/ticket row, minus
// the status icon — no separate Icon/glyph-column plumbing, since epic and
// ticket rows bake their own glyph into the Label string too. --all mode
// behaves the same as single-worktree: it just interleaves every worktree's
// epics into this one grouping, each epic row labeled with its worktree
// (renderEpicRow).
func (m Model) sidebarRenderOpts(width int) tree.RenderOpts[sidebarNode] {
	groups := groupEpicIndexesBySection(m.epics)

	entries := m.sidebarTree.Entries()
	idxByID := make(map[string]int, len(entries))
	for i, e := range entries {
		idxByID[e.ID] = i
	}

	return tree.RenderOpts[sidebarNode]{
		AccentColor: ui.ColorBlue,
		Active:      m.preview.Focus == preview.FocusSidebar,
		Width:       width,
		EmptyLine:   ui.StyleDim.Render("  loading…"),
		Label: func(entry tree.Entry[sidebarNode]) string {
			switch entry.Value.kind {
			case nodeSection:
				glyph := m.icons().TriangleExpanded
				if m.sidebarTree.CollapsedIDs()[entry.ID] {
					glyph = m.icons().TriangleCollapsed
				}
				n := len(groups[entry.Value.section])
				if entry.Value.section == sectionArchived {
					n = m.archivedEpicCount
				}
				return fmt.Sprintf("%s %s", glyph, sectionHeaderStyle.Render(fmt.Sprintf("%s (%d)", sectionLabel(entry.Value.section), n)))
			case nodeLoading:
				return ui.StyleDim.Render("loading…")
			case nodeLoadError:
				return ui.StyleError.Render("failed to load: " + m.archivedLazy.Err().Error())
			case nodeEpic:
				r, _ := rowFromEntry(entry)
				return m.renderEpicRow(m.epicAt(r))
			default: // nodeTicket
				r, _ := rowFromEntry(entry)
				return m.renderTicketRow(m.epicAt(r), r, idxByID[entry.ID])[0]
			}
		},
	}
}

func sectionLabel(section sidebarSection) string {
	switch section {
	case sectionOpen:
		return "Open epics"
	case sectionMapsInProgress:
		return "Maps in progress"
	case sectionOpenMaps:
		return "Open maps"
	case sectionComplete:
		return "Complete epics (unmerged)"
	case sectionMerged:
		return "Merged epics"
	case sectionCompleteMaps:
		return "Complete maps"
	default:
		return "Archived epics"
	}
}

func (m Model) renderEpicRow(epic tickets.Epic) string {
	glyph := m.icons().TriangleExpanded
	if m.isCollapsed(epic) {
		glyph = m.icons().TriangleCollapsed
	}
	line := fmt.Sprintf("%s %s%s (%d done / %d)", glyph, epic.Name, m.sessionMarker(epic.Name), epic.DoneCount(), epic.TotalCount())
	// Dimming tracks "every ticket done", not the current collapse toggle —
	// a fully-done epic stays dimmed even if manually expanded, and a
	// manually-collapsed in-progress epic doesn't borrow its dimming.
	if epic.AllDone() {
		line = rowfmt.StatusDoneStyle.Render(line)
	}
	if dur, ok := epic.CompletionDuration(); ok {
		text := "took " + rowfmt.FormatDuration(dur)
		if cost := queue.EpicCost(epic); cost > 0 {
			text += " " + tickets.FormatCost(cost)
		}
		line = rowfmt.AppendRowMetrics(line, text, rowfmt.MetricsLineStyle)
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
// sitting left of the checkbox, reflects r.expanded. A done ticket's line
// ends with the same elapsed/token metrics as the former standalone
// ralph-loop view, appended dim italic. Returns a single-element slice today
// (every branch renders exactly one physical line); the []string return
// keeps the multi-line-row seam open for a future ticket without another
// signature change at every call site.
func (m Model) renderTicketRow(epic tickets.Epic, r row, rowIdx int) []string {
	t := epic.Tickets[r.ticketIdx]
	status := rowfmt.TicketStatus(epic, t)

	triangle := strings.Repeat(" ", rowfmt.TriangleColumnWidth(m.icons())) + " "
	if r.hasChildren {
		glyph := m.icons().TriangleExpanded
		if !r.expanded {
			glyph = m.icons().TriangleCollapsed
		}
		triangle = glyph + " "
	}

	icon, style := rowfmt.StatusIconAndStyle(m.icons(), status)

	matched, _ := m.searchMatch(rowIdx)
	searchDim := m.search.HasQuery() && !matched
	doneDim := status.Terminal()

	title := rowfmt.RowTitle(m.icons(), t)
	if t.ShowsCommitlessSuffix() {
		title += " (commitless)"
	}
	titleStyle := lipgloss.NewStyle()
	if !matched {
		if doneDim {
			titleStyle = rowfmt.StatusDoneStyle
		} else if searchDim {
			titleStyle = ui.StyleDim
		}
	}
	if searchDim {
		style = ui.StyleDim
	}

	line := triangle + style.Render(icon)
	if m.rowHasSuggestedActions(epic, t, r.archived) {
		badgeStyle := rowfmt.SuggestedActionBadgeStyle
		if searchDim {
			badgeStyle = ui.StyleDim
		}
		line += " " + badgeStyle.Render(m.icons().SuggestedAction)
	}
	line += " " + titleStyle.Render(title) + m.sessionMarker(t.Path)
	if suffix := rowfmt.BlockedBySuffix(epic, t, status); suffix != "" {
		suffixStyle := rowfmt.BlockedBySuffixStyle
		if searchDim {
			suffixStyle = ui.StyleDim
		}
		line += " " + suffixStyle.Render(suffix)
	}
	if t.ElapsedTime <= 0 && t.ActualContextWindow <= 0 && t.ActualCost <= 0 {
		return []string{line}
	}
	metrics := rowfmt.FormatMetricsLine(t.ElapsedTime, t.ActualContextWindow, t.ActualCost, t.HadCodex)
	if searchDim {
		return []string{rowfmt.AppendRowMetrics(line, metrics, ui.StyleDim)}
	}
	return []string{rowfmt.AppendRowMetrics(line, metrics, style)}
}

// sessionMarker is " <glyph>" when a live interactive session's Session
// ticket is address, so nobody starts a second session on it; "" otherwise.
func (m Model) sessionMarker(address string) string {
	for _, s := range m.vm.Sessions {
		if s.Ticket == address && s.Status.State != wire.SessionExited {
			return " " + sessionMarkerStyle.Render(m.icons().SessionLive)
		}
	}
	return ""
}

func (m Model) icons() ui.IconSet {
	return ui.Icons(m.settings.UseNerdFontIcons)
}
