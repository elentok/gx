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
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/ui/tree"
)

var (
	sessionMarkerStyle = lipgloss.NewStyle().Foreground(ui.ColorTeal)

	sectionHeaderStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
)

// sidebarEnvFor snapshots what the sidebar reads from the rest of the tab.
func sidebarEnvFor(m Model) sidebarEnv {
	return sidebarEnv{
		query:       m.search.Query(),
		hasQuery:    m.search.HasQuery(),
		icons:       m.icons(),
		active:      m.preview.Focus == preview.FocusSidebar,
		loaded:      m.loaded,
		olderServer: m.olderServer,
		sessionMarker: func(address string) string {
			for _, s := range m.vm.Sessions {
				if s.Ticket == address && s.Status.State != wire.SessionExited {
					return " " + sessionMarkerStyle.Render(m.icons().SessionLive)
				}
			}
			return ""
		},
		hasSuggestedActions: func(epic tickets.Epic, ticket tickets.Ticket, archived bool) bool {
			return m.menus.HasSuggestedActions(m.menuEnv(), epic, ticket, archived)
		},
		pendingText: func(ticketPath string) (string, bool) {
			p, ok := m.vm.PendingRowFor(ticketPath)
			if !ok {
				return "", false
			}
			if p.Reason != "" {
				return p.Verdict + ": " + p.Reason, true
			}
			return p.Verdict, true
		},
	}
}

// body renders the sidebar panel's body lines. The tree's entries are empty
// before the first epicsLoadedMsg arrives, and also after a load with no
// epics at all (empty sections are hidden), so RenderOpts.EmptyLine covers the
// pre-load "loading…" state and the zero-epics-after-load "no .scratch/
// directory found" message keeps its own short-circuit ahead of the
// RenderLines call. That short-circuit only applies when there's truly nothing
// to show: a non-zero archivedEpicCount means the Archived section (ticket 04)
// still has something to render, so an empty active `.scratch/` alone must not
// suppress the whole tree.
func (s sidebar) body(env sidebarEnv, viewportH, width int) []string {
	if env.olderServer {
		return []string{ui.StyleWarning.Render("  " + tabapi.OlderServerNotice)}
	}
	if env.loaded && len(s.epics) == 0 && s.archivedEpicCount == 0 {
		return []string{ui.StyleMuted.Render("  no .scratch/ directory found")}
	}
	// RenderLines' own height param is an outer-panel height, from which it
	// subtracts 2 internally (see ui/tree/render.go) — the same convention
	// ui/status and ui/commit's own tree.Model callers already rely on. +2
	// here cancels that back out so it renders exactly viewportH body
	// rows, matching what the (headerless) sidebar panel actually has room for.
	return s.tree.RenderLines(viewportH+2, s.renderOpts(env, width))
}

// renderOpts builds the tree.RenderOpts s.tree.RenderLines
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
func (s sidebar) renderOpts(env sidebarEnv, width int) tree.RenderOpts[sidebarNode] {
	groups := groupEpicIndexesBySection(s.epics)

	entries := s.tree.Entries()
	idxByID := make(map[string]int, len(entries))
	for i, e := range entries {
		idxByID[e.ID] = i
	}

	return tree.RenderOpts[sidebarNode]{
		AccentColor: ui.ColorBlue,
		Active:      env.active,
		Width:       width,
		EmptyLine:   ui.StyleDim.Render("  loading…"),
		Label: func(entry tree.Entry[sidebarNode]) string {
			switch entry.Value.kind {
			case nodeSection:
				glyph := env.icons.TriangleExpanded
				if s.tree.CollapsedIDs()[entry.ID] {
					glyph = env.icons.TriangleCollapsed
				}
				n := len(groups[entry.Value.section])
				if entry.Value.section == sectionArchived {
					n = s.archivedEpicCount
				}
				return fmt.Sprintf("%s %s", glyph, sectionHeaderStyle.Render(fmt.Sprintf("%s (%d)", sectionLabel(entry.Value.section), n)))
			case nodeLoading:
				return ui.StyleDim.Render("loading…")
			case nodeLoadError:
				return ui.StyleError.Render("failed to load: " + s.archivedLazy.Err().Error())
			case nodeEpic:
				r, _ := rowFromEntry(entry)
				return s.renderEpicRow(env, s.epicAt(r))
			default: // nodeTicket
				r, _ := rowFromEntry(entry)
				return s.renderTicketRow(env, s.epicAt(r), r, idxByID[entry.ID])[0]
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

func (s sidebar) renderEpicRow(env sidebarEnv, epic tickets.Epic) string {
	glyph := env.icons.TriangleExpanded
	if s.isCollapsed(epic) {
		glyph = env.icons.TriangleCollapsed
	}
	line := fmt.Sprintf("%s %s%s (%d done / %d)", glyph, epic.Name, env.sessionMarker(epic.Name), epic.DoneCount(), epic.TotalCount())
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
func (s sidebar) renderTicketRow(env sidebarEnv, epic tickets.Epic, r row, rowIdx int) []string {
	icons := env.icons
	t := epic.Tickets[r.ticketIdx]
	status := rowfmt.TicketStatus(epic, t)

	triangle := strings.Repeat(" ", rowfmt.TriangleColumnWidth(icons)) + " "
	if r.hasChildren {
		glyph := icons.TriangleExpanded
		if !r.expanded {
			glyph = icons.TriangleCollapsed
		}
		triangle = glyph + " "
	}

	icon, style := rowfmt.StatusIconAndStyle(icons, status)

	matched, _ := s.tree.SearchMatch(rowIdx)
	searchDim := env.hasQuery && !matched
	doneDim := status.Terminal()

	title := rowfmt.RowTitle(icons, t)
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
	if env.hasSuggestedActions(epic, t, r.archived) {
		badgeStyle := rowfmt.SuggestedActionBadgeStyle
		if searchDim {
			badgeStyle = ui.StyleDim
		}
		line += " " + badgeStyle.Render(icons.SuggestedAction)
	}
	line += " " + titleStyle.Render(title) + env.sessionMarker(t.Path)
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

func (m Model) icons() ui.IconSet {
	return ui.Icons(m.settings.UseNerdFontIcons)
}
