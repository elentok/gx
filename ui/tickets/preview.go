package tickets

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

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
// padding, height less its header row.
func (m Model) previewInnerSize(previewW, h int) (width, height int) {
	width = max(previewW-2*previewPanelPaddingX, 1)
	height = max(h-previewPanelHeaderRow, 1)
	return
}

// previewLines renders the preview panel's body for a width x height
// content region (already excluding the panel's own padding/header, see
// normalView): the synthesized header/metadata chrome plus the
// glamour-rendered ticket body, scrolled through m.previewVP (kept in sync
// by syncPreviewViewport, see model_preview_focus.go) so the scroll
// indicator's dimensions come from the same source as what's displayed
// (mirrors ui/help's bodyWithScrollbar) and the scroll position persists
// across renders instead of resetting to the top every frame.
func (m Model) previewLines() []string {
	vp := m.previewVP

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
// an empty `.scratch/`) falls back to the tab's empty-preview placeholder.
// A ticket row gets a synthesized header line (icon + number + title), a
// metadata line (rendered status, type, unresolved blocked-by), a thin
// rule, then the ticket body rendered verbatim through glamour. An epic row
// gets its own header (name + optional [map] badge + open/total count)
// followed by its map.md body for a wayfinder-map epic, or nothing for a
// plain one.
func (m Model) previewContent(width int) string {
	r, ok := m.selectedRow()
	if !ok {
		return ui.StyleDim.Render("  no ticket selected")
	}
	if r.isEpic() {
		return previewEpicContent(m.epics[r.epicIdx], width)
	}

	epic := m.epics[r.epicIdx]
	t := epic.Tickets[r.ticketIdx]
	status := epic.RenderedStatus(t)

	var b strings.Builder
	b.WriteString(previewHeaderLine(m.icons(), status, t))
	if meta := previewMetaLine(epic, t, status); meta != "" {
		b.WriteString("\n")
		b.WriteString(meta)
	}
	b.WriteString("\n")
	b.WriteString(previewRuleStyle.Render(strings.Repeat("─", max(width, 0))))
	b.WriteString("\n")
	if t.ReadErr != "" {
		b.WriteString(statusErrorStyle.Render("  error reading ticket file: " + t.ReadErr))
	} else {
		b.WriteString(renderTicketMarkdown(t.Body, width))
	}
	return b.String()
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

func previewHeaderLine(icons ui.IconSet, status tickets.RenderedStatus, t tickets.Ticket) string {
	icon, style := statusIconAndStyle(icons, status)
	return "  " + style.Render(icon) + " " + ui.StyleBold.Render(fmt.Sprintf("#%d %s", t.Number, t.Title))
}

func previewMetaLine(epic tickets.Epic, t tickets.Ticket, status tickets.RenderedStatus) string {
	parts := []string{ui.StyleMuted.Render(status.Word())}
	if t.Type != "" {
		parts = append(parts, ui.StyleMuted.Render(t.Type))
	}
	if suffix := blockedBySuffix(epic, t, status); suffix != "" {
		parts = append(parts, blockedBySuffixStyle.Render(suffix))
	}
	return "  " + strings.Join(parts, "  ")
}
