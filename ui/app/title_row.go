package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui"
)

var (
	titleBgStyle    = lipgloss.NewStyle().Background(ui.ColorDeepBg)
	titleBrandStyle = titleBgStyle.Foreground(ui.ColorOverlay)
	titleSepStyle   = titleBgStyle.Foreground(ui.ColorSurface1)
	titleViewStyle  = titleBgStyle.Foreground(ui.ColorText).Bold(true)
	titleCaretStyle = titleBgStyle.Foreground(ui.ColorOverlay)
	titleHelpKey    = titleBgStyle.Foreground(ui.ColorSubtle).Bold(true)
	titleHelpWord   = titleBgStyle.Foreground(ui.ColorOverlay)
)

// span is a half-open column range [start, end) on the title row.
type span struct{ start, end int }

// titleRow is the rendered title row plus the click targets in it. Drawing and
// hit-testing both come from renderTitleRow, so they can't drift.
type titleRow struct {
	line string
	left span
	help span
}

// titleSegment is one styled piece of the row; width comes from the plain text.
type titleSegment struct {
	text  string
	style lipgloss.Style
}

func segmentsWidth(segs []titleSegment) int {
	w := 0
	for _, s := range segs {
		w += ansi.StringWidth(s.text)
	}
	return w
}

func renderSegments(segs []titleSegment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.style.Render(s.text))
	}
	return b.String()
}

// renderTitleRow builds the title row for width (<= 0 means unbounded).
func (m Model) renderTitleRow(width int) titleRow {
	left := []titleSegment{
		{" ", titleBgStyle},
		{"gx", titleBrandStyle},
		{" ", titleBgStyle},
		{"›", titleSepStyle},
		{" ", titleBgStyle},
		{m.activeViewLabel(), titleViewStyle},
		{" ", titleBgStyle},
		{"▾", titleCaretStyle},
	}
	// Attention items (ticket 03) go between the left group and help.
	right := []titleSegment{
		{"?", titleHelpKey},
		{" ", titleBgStyle},
		{"help", titleHelpWord},
		{" ", titleBgStyle},
	}
	leftW := segmentsWidth(left)
	rightW := segmentsWidth(right)
	gap := 1
	if width > leftW+rightW {
		gap = width - leftW - rightW
	}
	row := titleRow{
		line: renderSegments(left) + titleBgStyle.Render(strings.Repeat(" ", gap)) + renderSegments(right),
		// The leading margin is not part of the click target.
		left: span{start: 1, end: leftW},
		help: span{start: leftW + gap, end: leftW + gap + rightW - 1},
	}
	if width > 0 {
		row.line = ansi.Truncate(row.line, width, "")
		row.left = row.left.clip(width)
		row.help = row.help.clip(width)
	}
	return row
}

func (s span) clip(width int) span {
	s.start = min(s.start, width)
	s.end = min(s.end, width)
	return s
}

// activeViewLabel names the page on screen: the top of the history stack on a
// drill-down, else the live tab.
func (m Model) activeViewLabel() string {
	tab := m.activePage().viewState.Tab
	for _, spec := range m.tabSpecs() {
		if spec.id == tab {
			return spec.label
		}
	}
	return string(tab)
}

func (m Model) blankChromeRow(width int) string {
	return titleBgStyle.Render(strings.Repeat(" ", max(width, 0)))
}
