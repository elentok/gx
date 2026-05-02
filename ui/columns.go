package ui

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type FixedColumn struct {
	Text  string
	Width int
	Style lipgloss.Style
}

// RenderFixedColumns renders ANSI-safe fixed-width columns and joins them into
// one row. Each column is truncated and padded to its exact width.
func RenderFixedColumns(cols []FixedColumn) string {
	rendered := make([]string, 0, len(cols))
	for _, col := range cols {
		if col.Width <= 0 {
			continue
		}
		cellStyle := lipgloss.NewStyle().Width(col.Width).MaxWidth(col.Width).Inline(true)
		cell := cellStyle.Render(ansi.Truncate(col.Text, col.Width, "…"))
		rendered = append(rendered, col.Style.Render(cell))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}
