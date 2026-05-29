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
	FileIcon         func(entry Entry[T]) string
	FileLabel        func(entry Entry[T]) string
	MetaText         func(entry Entry[T]) string
	RowColor         func(entry Entry[T]) string
	Faint            func(entry Entry[T]) bool
}

func (m Model[T]) RenderLines(height int, opts RenderOpts[T]) []string {
	innerH := maxInt(1, height-2)
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
	return lines
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
