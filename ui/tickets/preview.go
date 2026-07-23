package tickets

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
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
// glamour-rendered ticket body, scrolled through a viewport so the scroll
// indicator's dimensions come from the same source as what's displayed
// (mirrors ui/help's bodyWithScrollbar).
func (m Model) previewLines(width, height int) []string {
	contentW := max(width-previewScrollbarGutter, 1)

	vp := viewport.New(viewport.WithWidth(contentW), viewport.WithHeight(height))
	vp.SetContent(m.previewContent(contentW))

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

// previewContent builds the selected ticket's preview: a synthesized
// header line (icon + number + title), a metadata line (rendered status,
// type, unresolved blocked-by), a thin rule, then the ticket body rendered
// verbatim through glamour. Selecting an epic row or nothing at all falls
// back to the tab's empty-preview placeholder (a proper epic preview is
// ticket 06's concern).
func (m Model) previewContent(width int) string {
	r, ok := m.selectedRow()
	if !ok || r.isEpic() {
		return ui.StyleDim.Render("  no ticket selected")
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
	b.WriteString(renderTicketMarkdown(t.Body, width))
	return b.String()
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
