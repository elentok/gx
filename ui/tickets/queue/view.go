package queue

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/tickets/preview"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

func (m QueueModel) View() tea.View {
	if !m.ready {
		return ui.NewHoverView("\n  Initializing…")
	}
	if m.loaded {
		// The checked set can change between Update calls with no queueEpicsLoadedMsg/
		// clampSelected in between, so this must be re-evaluated on every
		// render rather than only on the events clampSelected's own doc
		// comment lists. BuildEntriesCached (queue/rows.go) reuses the
		// previous tree when the checked set (among its other inputs) actually
		// didn't change, rather than always rebuilding from scratch.
		m.rows.Tree.SetEntries(m.rows.BuildEntriesCached(m.rowsEnv()))
	}
	sidebarW, previewW := m.preview.PanelWidths(m.width)
	sidebarH, previewH := m.preview.PanelHeights(m.width, m.layout().contentHeight())

	queueView := ui.RenderPanel(ui.PanelOptionsFor(
		sidebarW, sidebarH, m.header().Title(), "", m.queueBody(m.layout().bodyWidth()), m.preview.Focus == preview.FocusSidebar, ui.ColorBlue, nil, true,
	))
	previewView := ui.RenderPanel(ui.PanelOptionsFor(
		previewW, previewH, "Preview", preview.SearchMatchStatus(m.preview.Search), preview.RenderViewportWithScrollbar(m.preview.VP), m.preview.Focus == preview.FocusPreview, ui.ColorBlue, nil, false,
	))

	content := m.preview.JoinPanels(m.width, sidebarW, sidebarH, queueView, previewView)

	if m.menus.Menu.IsOpen {
		content = m.menus.Menu.Overlay(content, m.menuAnchor(), m.width, m.height)
	} else if m.confirm.IsOpen {
		content = ui.OverlayCenter(content, m.confirm.View(m.width), m.width, m.height)
	}
	if m.harnessPicker.Open {
		content = ui.OverlayCenter(content, m.harnessPicker.View(m.width), m.width, m.height)
	}
	if m.watch.IsOpen() {
		content = ui.OverlayCenter(content, m.watch.View(), m.width, m.height)
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
	return ui.NewHoverView(content)
}

// queueBody renders the Queue tab's panel body: the run-state banner
// (queueHeaderBodyLines, queue/header.go) as fixed lines, followed by
// m.rows.Tree's own windowed rendering — mirroring the Tickets tab's
// sidebarBody split between its own pre-load short-circuit and
// sidebarTree.RenderLines.
func (m QueueModel) queueBody(width int) []string {
	// A server down before the first load never loads, so its banner must
	// not wait behind "loading…".
	if m.server.OlderServer {
		return []string{ui.StyleWarning.Render("  " + tabapi.OlderServerNotice)}
	}
	if !m.loaded && !m.server.Down() {
		return []string{ui.StyleDim.Render("  loading…")}
	}

	hdr := m.header()
	var lines []string
	for _, line := range hdr.BodyLines() {
		if strings.Contains(line, "\x1b") {
			// Already carries its own ANSI styling (e.g. a rendered key hint
			// via ui.StatusWithHints) — wrapping it in StyleHint again would
			// bleed one color's reset codes into the other's spans.
			lines = append(lines, "  "+line)
			continue
		}
		lines = append(lines, "  "+ui.StyleHint.Render(line))
	}

	finished := hdr.FinishedLines()
	treeH := m.layout().viewportHeight() - len(lines) - len(finished)
	lines = append(lines, m.rows.Tree.RenderLines(treeH+2, m.rows.RenderOpts(m.rowsEnv(), width))...)
	return append(lines, finished...)
}

func (m QueueModel) icons() ui.IconSet {
	return ui.Icons(m.settings.UseNerdFontIcons)
}
