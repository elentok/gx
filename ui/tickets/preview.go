package tickets

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

var previewRuleStyle = lipgloss.NewStyle().Foreground(ui.ColorSurface1)

// previewScrollbarGutter is the width reserved to the right of the preview
// body for the scroll indicator (1 gap + 1 bar). It is always reserved so
// the body's wrap width - and thus the layout - doesn't shift depending on
// whether the selected ticket's body actually overflows.
const previewScrollbarGutter = 2

// previewPanelPaddingX and previewPanelHeaderRow mirror
// ui.PanelOptionsFor's fixed PaddingX: 1, PaddingY: 0 for every panel this
// tab renders - needed here to size the preview viewport to exactly the
// body area RenderPanel will paint into.
const (
	previewPanelPaddingX  = 1
	previewPanelHeaderRow = 1
)

// previewInnerSize returns the preview panel's usable content width/height
// for a given outer panel size: width less the panel's own horizontal
// padding, height less its header row. A free function (rather than a Model
// method) so the flat ralph-loop TUI (see flat.go) can share it without a
// tree-shaped Model of its own.
func previewInnerSize(previewW, h int) (width, height int) {
	width = max(previewW-2*previewPanelPaddingX, 1)
	height = max(h-previewPanelHeaderRow, 1)
	return
}

// renderViewportWithScrollbar pairs a viewport's currently visible lines
// with its scroll indicator. Free function shared with the flat ralph-loop
// TUI's own preview panel (see flat.go).
func renderViewportWithScrollbar(vp viewport.Model) []string {
	body := strings.Split(vp.View(), "\n")
	bar := ui.RenderScrollbar(vp.Height(), vp.TotalLineCount(), vp.VisibleLineCount(), vp.YOffset())
	if bar == "" {
		return body
	}

	barLines := strings.Split(bar, "\n")
	lines := make([]string, len(body))
	for i, line := range body {
		barSeg := ""
		if i < len(barLines) {
			barSeg = barLines[i]
		}
		lines[i] = line + " " + barSeg
	}
	return lines
}

// previewContent builds the selected row's preview. Nothing selected (e.g.
// an empty `.scratch/`) falls back to the tab's empty-preview placeholder. A
// ticket row is rendered by renderTicketPreview, shared with the Queue tab's
// own preview pane (see queue_preview.go). An epic row gets its own header
// (name + optional [map] badge + open/total count) followed by its map.md
// body for a wayfinder-map epic, or nothing for a plain one. The second and
// third return values are the park-section scroll target line index and
// whether one exists — always false for an epic row.
func (m Model) previewContent(width int) (string, int, bool) {
	r, ok := m.selectedRow()
	if !ok {
		return ui.StyleDim.Render("  no ticket selected"), 0, false
	}
	if r.isEpic() {
		return previewEpicContent(m.epicAt(r), width), 0, false
	}

	epic := m.epicAt(r)
	t := epic.Tickets[r.ticketIdx]
	return renderTicketPreview(epic, t, width)
}

// renderTicketPreview builds one ticket row's preview: its formatted
// frontmatter block (renderFrontmatterBlock), a thin rule, then the ticket
// body rendered verbatim through glamour (or a read-error message), with the
// ticket's park section (if any — see highlightParkSection) highlighted in
// its severity color. Shared verbatim by the Tickets tab (previewContent
// above) and the Queue tab (queue_preview.go) so both tabs' previews stay
// identical rather than drifting into two similar-but-slightly-different
// implementations. The second and third return values are the park
// section's line index within the returned content and whether one was
// found, for the caller to auto-scroll to.
func renderTicketPreview(epic tickets.Epic, t tickets.Ticket, width int) (string, int, bool) {
	status := epic.RenderedStatus(t)

	var b strings.Builder
	b.WriteString(renderFrontmatterBlock(t, status, width))
	b.WriteString("\n")
	b.WriteString(previewRuleStyle.Render(strings.Repeat("─", max(width, 0))))
	b.WriteString("\n")
	if t.ReadErr != "" {
		b.WriteString(statusErrorStyle.Render("  error reading ticket file: " + t.ReadErr))
		return b.String(), 0, false
	}

	prefixLines := strings.Count(b.String(), "\n")
	body, target, ok := highlightParkSection(renderTicketMarkdown(t.Body, width), status)
	b.WriteString(body)
	if ok {
		target += prefixLines
	}
	return b.String(), target, ok
}

// highlightParkSection finds status's park heading ("## Needs Answer"/"##
// Needs Repair", the literal text MarkNeedsAnswerWithReasonAndStub/
// MarkNeedsRepairWithReason append — see ralphloop/claim.go) within rendered
// (glamour output, so headings survive verbatim as their own line — see
// ticketGlamourStyle's H2 prefix in glamour.go) and re-styles that line
// through the section's end (the next "## " heading, or content end) in the
// status's severity color: orange for needs-answer, red for needs-repair —
// deliberately different from the row-icon colors (ColorYellow/ColorRed in
// view.go), since ticket 20 wants orange for needs-answer in the preview,
// not yellow. Mirrors highlightPreviewLine's approach of re-styling the
// ANSI-stripped text in place rather than fighting glamour's own ANSI.
// Returns rendered unchanged with ok=false for any other status or when the
// heading isn't found.
func highlightParkSection(rendered string, status tickets.RenderedStatus) (out string, target int, ok bool) {
	var heading string
	var style lipgloss.Style
	switch status {
	case tickets.StatusNeedsAnswer:
		heading = needsAnswerHeading
		style = lipgloss.NewStyle().Foreground(ui.ColorOrange)
	case tickets.StatusNeedsRepair:
		heading = needsRepairHeading
		style = lipgloss.NewStyle().Foreground(ui.ColorRed)
	default:
		return rendered, 0, false
	}

	lines := strings.Split(rendered, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(ansi.Strip(line)) == heading {
			start = i
			break
		}
	}
	if start == -1 {
		return rendered, 0, false
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(ansi.Strip(lines[i])), "## ") {
			end = i
			break
		}
	}
	for i := start; i < end; i++ {
		lines[i] = style.Render(ansi.Strip(lines[i]))
	}
	return strings.Join(lines, "\n"), start, true
}

// previewEpicContent renders an epic row's preview: a header line, plus -
// for a wayfinder-map epic only - a rule and its map.md body rendered
// through the same glamour path as a ticket body. A plain epic (no map.md)
// has no single representative file to preview, so it's header-only.
func previewEpicContent(epic tickets.Epic, width int) string {
	header := previewEpicHeaderLine(epic)
	if !epic.IsMap {
		return header
	}

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(previewRuleStyle.Render(strings.Repeat("─", max(width, 0))))
	b.WriteString("\n")
	b.WriteString(renderTicketMarkdown(epic.MapBody, width))
	return b.String()
}

func previewEpicHeaderLine(epic tickets.Epic) string {
	line := "  " + ui.StyleBold.Render(epic.Name)
	if epic.IsMap {
		line += " " + ui.StyleMuted.Render("[map]")
	}
	line += " " + ui.StyleMuted.Render(fmt.Sprintf("(%d done / %d)", epic.DoneCount(), epic.TotalCount()))
	return line
}
