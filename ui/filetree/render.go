package filetree

import (
	"image/color"
	"strings"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type RenderOpts[T any] struct {
	AccentColor      color.Color
	Active           bool
	EmptyLine        string
	UseNerdFontIcons bool
	Width            int
	FileIcon         func(entry Entry[T]) string
	FileLabel        func(entry Entry[T]) string
	MetaText         func(entry Entry[T]) string
	RowColor         func(entry Entry[T]) string
	Faint            func(entry Entry[T]) bool
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
			lines = append(lines, m.renderEntry(row.index, row.entry, opts, row.index == m.SelectedIndex()))
		}
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
	bar := ui.RenderScrollbar(height, len(m.entries), height, m.ScrollOffset())
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
	offset := m.ScrollOffset()
	end := offset + innerH
	if end > total {
		end = total
	}
	rows := make([]visibleEntry[T], 0, maxInt(0, end-offset))
	for i := offset; i < end; i++ {
		rows = append(rows, visibleEntry[T]{index: i, entry: m.entries[i]})
	}
	return rows
}

func (m Model[T]) renderEntry(index int, entry Entry[T], opts RenderOpts[T], selected bool) string {
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
	line := mark + strings.Repeat("  ", entry.Depth) + meta + sep + name
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
	if entry.Kind == EntryDir {
		icons := ui.Icons(opts.UseNerdFontIcons)
		symbol := icons.FolderOpen
		if !entry.Expanded {
			symbol = icons.FolderClosed
		}
		return symbol + " " + entry.DisplayName + "/"
	}
	label := entry.DisplayName
	if opts.FileLabel != nil {
		label = opts.FileLabel(entry)
	}
	if opts.FileIcon == nil {
		return label
	}
	icon := strings.TrimSpace(opts.FileIcon(entry))
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
