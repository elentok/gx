package prs

import (
	"errors"
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

	closedSectionHeaderStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	closedMergedStyle        = lipgloss.NewStyle().Foreground(ui.ColorGreen)
	closedUnmergedStyle      = lipgloss.NewStyle().Foreground(ui.ColorRed).Faint(true)
	closedTitleStyle         = lipgloss.NewStyle().Foreground(ui.ColorText)
	closedDateStyle          = lipgloss.NewStyle().Foreground(ui.ColorSubtle).Italic(true)
)

// facetIndent lines the facet line up under the row's title, matching the
// 2-wide marker column above it (mirrors ui/log's subjectIndentNoGraph).
const facetIndent = "  "

func (m Model) visibleLines() []string {
	lines := m.openListLines()
	lines = append(lines, m.closedSectionLines()...)
	return lines
}

// openListLines renders the open-PR list's loading/error/empty/row states.
// Rendering the closed section is independent of this — it always renders
// below regardless of the open list's own state (see closedSectionLines),
// and the two sections load concurrently (issues/09-load-time-batched-fetch.md).
func (m Model) openListLines() []string {
	if !m.openLoaded {
		return []string{ui.StyleMuted.Render("loading…")}
	}
	if m.err != nil {
		return m.errorLines(m.err)
	}
	if len(m.prs) == 0 {
		if m.anyPRs {
			return []string{ui.StyleMuted.Render("no open PRs")}
		}
		return []string{ui.StyleMuted.Render("no PRs found")}
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

// closedSectionLines renders the "Closed (last 2 weeks)" section: a header
// followed by one line per recently-closed PR (marker, title, closed date —
// no facets), or a muted empty state when there are none. Renders
// unconditionally once loaded, independent of the open-PR list's own
// state (empty or erroring).
func (m Model) closedSectionLines() []string {
	lines := []string{"", closedSectionHeaderStyle.Render("── Closed (last 2 weeks) ──")}
	if !m.closedLoaded {
		return append(lines, ui.StyleMuted.Render("loading…"))
	}
	if len(m.closedPRs) == 0 {
		return append(lines, ui.StyleMuted.Render("no recently closed PRs"))
	}

	innerW := max(1, m.width-4)
	for _, pr := range m.closedPRs {
		lines = append(lines, m.renderClosedRow(pr, innerW))
	}
	return lines
}

func (m Model) renderClosedRow(pr git.ClosedPR, width int) string {
	icons := ui.Icons(m.settings.UseNerdFontIcons)

	marker := icons.Check
	markerStyle := closedMergedStyle
	if !pr.IsMerged() {
		marker = icons.Close
		markerStyle = closedUnmergedStyle
	}
	markerCol := ui.RenderFixedColumns([]ui.FixedColumn{{Text: marker, Width: 2, Style: markerStyle}})

	date := closedDateStyle.Render(ui.RelativeTimeCompact(pr.ClosedAt))

	markerW := ansi.StringWidth(markerCol)
	dateW := ansi.StringWidth(date)
	gap := 1

	titleW := max(1, width-markerW-gap-dateW)
	title := ansi.Truncate(closedTitleStyle.Render(pr.Title), titleW, "…")
	titleActualW := ansi.StringWidth(title)
	if titleActualW < titleW {
		title += strings.Repeat(" ", titleW-titleActualW)
	}

	return markerCol + title + " " + date
}

// renderRow renders one PR as two physical lines: the subject line (marker,
// number, draft badge, title, age) and an indented facet line below it
// (CI/approval/mergeable/comment icons), each truncated/padded to width and
// background-styled uniformly so a selection highlight covers the full row.
// errorLines renders a tailored inline message for gh's two most common
// failure modes, falling back to gh's raw wrapped message for everything
// else (network, rate limit, no GitHub remote, ...).
func (m Model) errorLines(err error) []string {
	var prErr *git.PRListError
	if errors.As(err, &prErr) {
		switch prErr.Kind {
		case git.PRListErrorGHNotInstalled:
			return []string{
				ui.StyleWarning.Render("gh not found"),
				ui.StyleMuted.Render("install: https://cli.github.com"),
			}
		case git.PRListErrorUnauthenticated:
			return []string{
				ui.StyleWarning.Render("gh is not authenticated"),
				ui.StyleMuted.Render("run: gh auth login"),
			}
		}
	}
	return []string{ui.StyleWarning.Render("error: " + err.Error())}
}

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
		return facetPendingStyle.Render(icons.CIRunning + " checking")
	case git.CIFailed:
		return facetFailedStyle.Render(icons.Close + " failing")
	case git.CIPassed:
		return facetPassedStyle.Render(icons.Check + " passing")
	default:
		return facetNoneStyle.Render(icons.Dot)
	}
}

func renderApprovalFacet(icons ui.IconSet, state git.ApprovalState) string {
	switch state {
	case git.ApprovalApproved:
		return facetPassedStyle.Render(icons.Check + " approved")
	case git.ApprovalChangesRequested:
		return facetFailedStyle.Render(icons.Close + " changes requested")
	case git.ApprovalCommentedOnly:
		return facetPendingStyle.Render(icons.Commented + " commented")
	default:
		return facetNoneStyle.Render(icons.Dot + " review needed")
	}
}

func renderMergeableFacet(icons ui.IconSet, state git.MergeableState) string {
	switch state {
	case git.MergeableConflicting:
		return facetConflictStyle.Render(icons.Warning + " conflicts")
	case git.MergeableChecking:
		return facetCheckingStyle.Render(icons.Ellipsis + " checking")
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
