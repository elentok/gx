package tree

import (
	"image/color"
	"strings"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type RenderOpts[T any] struct {
	AccentColor color.Color
	Active      bool
	EmptyLine   string
	Width       int
	// Icon renders a per-row prefix (e.g. an expand/collapse glyph). Optional.
	Icon func(entry Entry[T]) string
	// Label renders the row's text content.
	Label    func(entry Entry[T]) string
	MetaText func(entry Entry[T]) string
	RowColor func(entry Entry[T]) string
	Faint    func(entry Entry[T]) bool
}

func (m Model[T]) RenderLines(height int, opts RenderOpts[T]) []string {
	innerH := maxInt(1, height-2)
	var searchLines []string
	if m.search.InputFocused() && opts.Width > 0 {
		m.search.SetWidth(opts.Width)
		searchLines = strings.Split(m.search.View(), "\n")
		innerH = maxInt(0, innerH-len(searchLines))
	}
	entries := m.visibleEntries(innerH)
	lines := make([]string, 0, innerH)
	if len(entries) == 0 {
		lines = append(lines, opts.EmptyLine)
	} else {
		for _, row := range entries {
			lines = append(lines, m.renderEntryLines(row.index, row.entry, opts, row.index == m.SelectedIndex())...)
		}
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	lines = m.appendScrollbar(lines, innerH, opts)
	lines = append(lines, searchLines...)
	return lines
}

// appendScrollbar right-aligns a 2-column gutter (" " + glyph) onto each of
// the height entry rows, padding shorter rows out first so every glyph lands
// in the same column; the gutter renders blank when the entries fit without
// scrolling. Rows are padded to opts.Width-2 when a width is set, or to the
// widest row otherwise (e.g. when called from RequiredWidth).
func (m Model[T]) appendScrollbar(lines []string, height int, opts RenderOpts[T]) []string {
	bar := ui.RenderScrollbar(height, m.totalLines(), height, m.offsetLines())
	var barLines []string
	if bar != "" {
		barLines = strings.Split(bar, "\n")
	}
	padW := opts.Width - 2
	if padW <= 0 {
		padW = 0
		for _, line := range lines {
			if w := ansi.StringWidth(line); w > padW {
				padW = w
			}
		}
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		b := " "
		if i < len(barLines) {
			b = barLines[i]
		}
		pad := max(0, padW-ansi.StringWidth(line))
		out[i] = line + strings.Repeat(" ", pad) + " " + b
	}
	return out
}

func (m Model[T]) RequiredWidth(height int, opts RenderOpts[T]) int {
	required := 0
	for _, line := range m.RenderLines(height, opts) {
		if w := ansi.StringWidth(line); w > required {
			required = w
		}
	}
	return required
}

type visibleEntry[T any] struct {
	index int
	entry Entry[T]
}

func (m Model[T]) visibleEntries(innerH int) []visibleEntry[T] {
	total := len(m.entries)
	if total == 0 || innerH <= 0 {
		return nil
	}
	start, end := m.list.VisibleRangeLines(total, m.entryLineHeight, innerH)
	rows := make([]visibleEntry[T], 0, maxInt(0, end-start))
	for i := start; i < end; i++ {
		rows = append(rows, visibleEntry[T]{index: i, entry: m.entries[i]})
	}
	return rows
}

// totalLines is the tree's full content height in physical lines, used by
// appendScrollbar to keep the thumb proportional once entries have
// different heights.
func (m Model[T]) totalLines() int {
	total := 0
	for i := range m.entries {
		total += m.entryLineHeight(i)
	}
	return total
}

// offsetLines is the physical-line position of the current scroll offset:
// the summed line height of every entry above it.
func (m Model[T]) offsetLines() int {
	offset := 0
	for i := 0; i < m.ScrollOffset() && i < len(m.entries); i++ {
		offset += m.entryLineHeight(i)
	}
	return offset
}

func (m Model[T]) renderEntryLines(index int, entry Entry[T], opts RenderOpts[T], selected bool) []string {
	mark := " "
	if selected {
		mark = lipgloss.NewStyle().Foreground(opts.AccentColor).Render("▌")
	}

	colorStyle := lipgloss.NewStyle()
	if opts.RowColor != nil {
		colorStyle = colorStyle.Foreground(lipgloss.Color(opts.RowColor(entry)))
	}
	faint := opts.Faint != nil && opts.Faint(entry)
	if faint {
		colorStyle = colorStyle.Faint(true)
	}

	indent := strings.Repeat("  ", entry.Depth)

	meta := ""
	if opts.MetaText != nil {
		meta = colorStyle.Render(opts.MetaText(entry))
	}
	name := renderLabel(entry, opts)
	if matched, current := m.SearchMatch(index); matched {
		name = search.Highlight(name, m.search.Query(), current)
	}
	name = colorStyle.Render(name)

	sep := " "
	if strings.TrimSpace(meta) == "" {
		sep = ""
	}
	lines := make([]string, 0, entry.lineCount())
	lines = append(lines, m.styleEntryLine(mark+indent+meta+sep+name, selected, faint, opts))
	for _, body := range entry.Body {
		lines = append(lines, m.styleEntryLine(mark+indent+colorStyle.Render(body), selected, faint, opts))
	}
	return lines
}

// styleEntryLine applies the selection/width styling shared by an entry's
// primary line and its Body continuation lines.
func (m Model[T]) styleEntryLine(line string, selected, faint bool, opts RenderOpts[T]) string {
	if selected && !faint {
		line = lipgloss.NewStyle().Bold(true).Render(line)
	}
	if selected && opts.Active && line != "" {
		line = ui.RenderRowHighlight(line)
	}
	if opts.Width > 0 {
		line = ansi.Truncate(line, opts.Width-2, "")
	}
	return line
}

func renderLabel[T any](entry Entry[T], opts RenderOpts[T]) string {
	label := ""
	if opts.Label != nil {
		label = opts.Label(entry)
	}
	if opts.Icon == nil {
		return label
	}
	icon := strings.TrimSpace(opts.Icon(entry))
	if icon == "" {
		return label
	}
	return icon + " " + label
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
