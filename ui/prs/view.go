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
)

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

	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(m.prs[i], i == sel, innerW))
	}
	return lines
}

func (m Model) renderRow(pr git.PR, selected bool, width int) string {
	number := prsNumberStyle.Render("#" + strconv.Itoa(pr.Number))
	age := prsAgeStyle.Render(ui.RelativeTimeCompact(pr.UpdatedAt))

	draft := ""
	if pr.IsDraft {
		draft = ui.RenderBadgeText("DRAFT", ui.ColorYellow) + " "
	}

	numberW := ansi.StringWidth(number)
	ageW := ansi.StringWidth(age)
	draftW := ansi.StringWidth(draft)
	gap := 1

	titleW := max(1, width-numberW-gap-draftW-gap-ageW)
	title := ansi.Truncate(prsTitleStyle.Render(pr.Title), titleW, "…")
	titleActualW := ansi.StringWidth(title)
	if titleActualW < titleW {
		title += strings.Repeat(" ", titleW-titleActualW)
	}

	line := number + " " + draft + title + " " + age
	lineW := ansi.StringWidth(line)
	if lineW < width {
		line += strings.Repeat(" ", width-lineW)
	}

	if selected {
		return ui.RenderRowHighlight(line)
	}
	return line
}
