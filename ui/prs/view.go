package prs

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
)

var (
	prsNumberStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	prsTitleStyle  = lipgloss.NewStyle().Foreground(ui.ColorText)
	prsAgeStyle    = lipgloss.NewStyle().Foreground(ui.ColorSubtle).Italic(true)

	facetFailedStyle   = lipgloss.NewStyle().Foreground(ui.ColorRed)
	facetPassedStyle   = lipgloss.NewStyle().Foreground(ui.ColorGreen)
	facetPendingStyle  = lipgloss.NewStyle().Foreground(ui.ColorYellow)
	facetNoneStyle     = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	facetConflictStyle = lipgloss.NewStyle().Foreground(ui.ColorRed).Bold(true)
	facetCheckingStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle).Italic(true)

	markerReadyStyle   = lipgloss.NewStyle().Foreground(ui.ColorGreen)
	markerBlockedStyle = lipgloss.NewStyle().Foreground(ui.ColorRed)
	markerWaitingStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
)

// facetIndent lines the facet line up under the row's title, matching the
// 2-wide marker column above it (mirrors ui/log's subjectIndentNoGraph).
const facetIndent = "  "

func (m Model) visibleLines() []string {
	if !m.loaded {
		return []string{ui.StyleMuted.Render("loading…")}
	}
	if m.err != nil {
		return []string{ui.StyleWarning.Render("error: " + m.err.Error())}
	}
	if len(m.prs) == 0 {
		return []string{ui.StyleMuted.Render("no PRs")}
	}

	innerW := max(1, m.width-4)
	start, end := m.list.VisibleRange(len(m.prs), m.visibleH())
	sel := m.list.Selected()

	lines := make([]string, 0, (end-start)*2)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(m.prs[i], i == sel, innerW)...)
	}
	return lines
}

// renderRow renders one PR as two physical lines: the subject line (marker,
// number, draft badge, title, age) and an indented facet line below it
// (CI/approval/mergeable/comment icons), each truncated/padded to width and
// background-styled uniformly so a selection highlight covers the full row.
func (m Model) renderRow(pr git.PR, selected bool, width int) []string {
	facets := pr.Facets()
	rawLines := []string{
		m.renderSubjectLine(pr, facets, width),
		facetIndent + m.renderFacetLine(facets),
	}

	lines := make([]string, len(rawLines))
	for i, line := range rawLines {
		line = ansi.Truncate(line, width, "…")
		lineW := ansi.StringWidth(line)
		if lineW < width {
			line += strings.Repeat(" ", width-lineW)
		}
		if selected {
			line = ui.RenderRowHighlight(line)
		}
		lines[i] = line
	}
	return lines
}

func (m Model) renderSubjectLine(pr git.PR, facets git.Facets, width int) string {
	icons := ui.Icons(m.settings.UseNerdFontIcons)
	markerState := markerPushState(icons, facets.Marker())
	marker := ui.RenderFixedColumns([]ui.FixedColumn{{Text: markerState.Icon, Width: 2, Style: markerState.Style}})

	number := prsNumberStyle.Render("#" + strconv.Itoa(pr.Number))
	age := prsAgeStyle.Render(ui.RelativeTimeCompact(pr.UpdatedAt))

	draft := ""
	if pr.IsDraft {
		draft = ui.RenderBadgeText("DRAFT", ui.ColorYellow) + " "
	}

	markerW := ansi.StringWidth(marker)
	numberW := ansi.StringWidth(number)
	ageW := ansi.StringWidth(age)
	draftW := ansi.StringWidth(draft)
	gap := 1

	titleW := max(1, width-markerW-numberW-gap-draftW-gap-ageW)
	title := ansi.Truncate(prsTitleStyle.Render(pr.Title), titleW, "…")
	titleActualW := ansi.StringWidth(title)
	if titleActualW < titleW {
		title += strings.Repeat(" ", titleW-titleActualW)
	}

	return marker + number + " " + draft + title + " " + age
}

func (m Model) renderFacetLine(facets git.Facets) string {
	icons := ui.Icons(m.settings.UseNerdFontIcons)

	parts := []string{
		renderCIFacet(icons, facets.CI),
		renderApprovalFacet(icons, facets.Approval),
	}
	if mergeable := renderMergeableFacet(icons, facets.Mergeable); mergeable != "" {
		parts = append(parts, mergeable)
	}
	if comments := renderCommentFacet(icons, m.settings.UseNerdFontIcons, facets.CommentCount); comments != "" {
		parts = append(parts, comments)
	}
	return strings.Join(parts, " ")
}

// markerPushState renders a marker as a PushState-style {Icon, Label, Style}
// value, following ui/pushstate.go's convention for compact status icons.
func markerPushState(icons ui.IconSet, marker git.Marker) ui.PushState {
	switch marker {
	case git.MarkerGreen:
		return ui.PushState{Icon: icons.MarkerReady, Label: "ready to merge", Style: markerReadyStyle}
	case git.MarkerRed:
		return ui.PushState{Icon: icons.MarkerBlocked, Label: "blocked on you", Style: markerBlockedStyle}
	default:
		return ui.PushState{Icon: icons.MarkerWaiting, Label: "waiting on others", Style: markerWaitingStyle}
	}
}

func renderCIFacet(icons ui.IconSet, state git.CIState) string {
	switch state {
	case git.CIRunning:
		return facetPendingStyle.Render(icons.CIRunning)
	case git.CIFailed:
		return facetFailedStyle.Render(icons.Close)
	case git.CIPassed:
		return facetPassedStyle.Render(icons.Check)
	default:
		return facetNoneStyle.Render(icons.Dot)
	}
}

func renderApprovalFacet(icons ui.IconSet, state git.ApprovalState) string {
	switch state {
	case git.ApprovalApproved:
		return facetPassedStyle.Render(icons.Check)
	case git.ApprovalChangesRequested:
		return facetFailedStyle.Render(icons.Close)
	case git.ApprovalCommentedOnly:
		return facetPendingStyle.Render(icons.Commented)
	default:
		return facetNoneStyle.Render(icons.Dot)
	}
}

func renderMergeableFacet(icons ui.IconSet, state git.MergeableState) string {
	switch state {
	case git.MergeableConflicting:
		return facetConflictStyle.Render(icons.Warning)
	case git.MergeableChecking:
		return facetCheckingStyle.Render(icons.Ellipsis)
	default:
		return ""
	}
}

func renderCommentFacet(icons ui.IconSet, useNerdFont bool, count int) string {
	if count == 0 {
		return ""
	}
	text := strconv.Itoa(count) + "c"
	if useNerdFont {
		text = icons.Comment + " " + strconv.Itoa(count)
	}
	return facetNoneStyle.Render(text)
}
