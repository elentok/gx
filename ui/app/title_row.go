package app

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	ticketsui "github.com/elentok/gx/ui/tickets"
)

var (
	titleBgStyle    = lipgloss.NewStyle().Background(ui.ColorDeepBg)
	titleBrandStyle = titleBgStyle.Foreground(ui.ColorOverlay)
	titleSepStyle   = titleBgStyle.Foreground(ui.ColorSurface1)
	titleViewStyle  = titleBgStyle.Foreground(ui.ColorText).Bold(true)
	titleCaretStyle = titleBgStyle.Foreground(ui.ColorOverlay)
	titleHelpKey    = titleBgStyle.Foreground(ui.ColorSubtle).Bold(true)
	titleHelpWord   = titleBgStyle.Foreground(ui.ColorOverlay)

	titleAgentsWord  = titleBgStyle.Foreground(ui.ColorOverlay)
	titleAgentsCount = titleBgStyle.Foreground(ui.ColorRed).Bold(true)

	titleQueueCount   = titleBgStyle.Foreground(ui.ColorSubtle)
	titleQueueBarRest = titleBgStyle.Foreground(ui.ColorSurface1)
)

// queueBarCells is how many cells the title row's queue progress bar spans.
const queueBarCells = 7

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

// serverWords picks which of the server badge's word forms a shape shows.
type serverWords int

const (
	serverWordsFull serverWords = iota
	serverWordsShort
	serverWordsNone
)

func (b serverBadge) words(w serverWords) string {
	switch w {
	case serverWordsFull:
		return b.full
	case serverWordsShort:
		return b.short
	}
	return ""
}

// titleShape is one step of the drop order: what the row keeps.
type titleShape struct {
	shortHelp bool
	words     serverWords
	brand     bool
	caret     bool
	queueBar  bool
}

// titleDropOrder runs widest to narrowest; the first shape that fits wins.
// The queue icon and count, the agents count and the server icon are in every
// shape.
var titleDropOrder = []titleShape{
	{brand: true, caret: true, queueBar: true},
	{shortHelp: true, brand: true, caret: true, queueBar: true},
	{shortHelp: true, brand: true, caret: true},
	{shortHelp: true, words: serverWordsShort, brand: true, caret: true},
	{shortHelp: true, words: serverWordsNone, brand: true, caret: true},
	{shortHelp: true, words: serverWordsNone, caret: true},
	{shortHelp: true, words: serverWordsNone},
}

// renderTitleRow builds the title row for width (<= 0 means unbounded).
func (m Model) renderTitleRow(width int) titleRow {
	name := m.activeViewLabel()
	shape := titleDropOrder[0]
	for _, shape = range titleDropOrder {
		if width <= 0 || m.titleWidth(shape, name) <= width {
			break
		}
	}
	if over := m.titleWidth(shape, name) - width; width > 0 && over > 0 {
		name = ansi.Truncate(name, max(ansi.StringWidth(name)-over, 1), "…")
	}
	left, right, helpW := m.titleGroups(shape, name)
	leftW := segmentsWidth(left)
	rightW := segmentsWidth(right)
	gap := max(width-leftW-rightW, 1)
	helpStart := leftW + gap + rightW - helpW - 1
	row := titleRow{
		line: renderSegments(left) + titleBgStyle.Render(strings.Repeat(" ", gap)) + renderSegments(right),
		// The leading margin is not part of the click target.
		left: span{start: 1, end: leftW},
		help: span{start: helpStart, end: helpStart + helpW},
	}
	if width > 0 {
		row.line = ansi.Truncate(row.line, width, "")
		row.left = row.left.clip(width)
		row.help = row.help.clip(width)
	}
	return row
}

// titleWidth is the row's width in shape with the minimum one-column gap.
func (m Model) titleWidth(shape titleShape, name string) int {
	left, right, _ := m.titleGroups(shape, name)
	return segmentsWidth(left) + 1 + segmentsWidth(right)
}

// titleGroups returns the left group, the right group (attention items then
// help, with a trailing margin) and the width of the help item.
func (m Model) titleGroups(shape titleShape, name string) (left, right []titleSegment, helpW int) {
	sp := titleSegment{" ", titleBgStyle}
	left = []titleSegment{sp}
	if shape.brand {
		left = append(left, titleSegment{"gx", titleBrandStyle}, sp, titleSegment{"›", titleSepStyle}, sp)
	}
	left = append(left, titleSegment{name, titleViewStyle})
	if shape.caret {
		left = append(left, sp, titleSegment{"▾", titleCaretStyle})
	}

	icons := ui.Icons(m.settings.UseNerdFontIcons)
	if m.stream.queueOK {
		right = append(right, queueSegments(m.stream.queue, icons, shape.queueBar)...)
	}
	if n := m.stream.vm.ActionableCount(); n > 0 {
		right = append(right, titleSegment{icons.Agents, titleAgentsWord}, sp, titleSegment{strconv.Itoa(n), titleAgentsCount}, sp)
	}
	if b, ok := m.serverConn().badge(icons); ok {
		style := titleBgStyle.Foreground(b.color)
		right = append(right, titleSegment{b.icon, style}, sp)
		if words := b.words(shape.words); words != "" {
			right = append(right, titleSegment{words, style}, sp)
		}
	}
	help := []titleSegment{{"?", titleHelpKey}}
	if !shape.shortHelp {
		help = append(help, sp, titleSegment{"help", titleHelpWord})
	}
	right = append(append(right, help...), sp)
	return left, right, segmentsWidth(help)
}

// queueSegments draws the queue status: a mode icon, a progress bar when bar is
// set and the done/total count, with a trailing space.
func queueSegments(q ticketsui.QueueStatus, icons ui.IconSet, bar bool) []titleSegment {
	icon, color := icons.QueueRunning, ui.ColorGreen
	switch q.Mode {
	case server.ModePaused:
		icon, color = icons.QueuePaused, ui.ColorYellow
	case server.ModeDraining:
		icon, color = icons.QueueDraining, ui.ColorOrange
	}
	sp := titleSegment{" ", titleBgStyle}
	segs := []titleSegment{{icon, titleBgStyle.Foreground(color)}, sp}
	if bar {
		filled := q.Done * queueBarCells / max(q.Total, 1)
		segs = append(segs,
			titleSegment{strings.Repeat("▰", filled), titleBgStyle.Foreground(color)},
			titleSegment{strings.Repeat("▱", queueBarCells-filled), titleQueueBarRest},
			sp)
	}
	count := strconv.Itoa(q.Done) + "/" + strconv.Itoa(q.Total)
	return append(segs, titleSegment{count, titleQueueCount}, sp)
}

func (s span) clip(width int) span {
	s.start = min(s.start, width)
	s.end = min(s.end, width)
	return s
}

func (s span) contains(x int) bool {
	return x >= s.start && x < s.end
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
