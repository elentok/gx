package sidebar

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

type RenderableRow struct {
	Depth    int
	MetaRaw  string
	NameRaw  string
	Color    string
	Selected bool
	Faint    bool
}

func BuildVisibleRenderableRows[T any](entries []T, offset, innerH int, build func(index int, entry T) RenderableRow) []RenderableRow {
	total := len(entries)
	if total <= 0 || innerH <= 0 {
		return nil
	}
	end := offset + innerH
	if end > total {
		end = total
	}
	rows := make([]RenderableRow, 0, maxInt(0, end-offset))
	for i := offset; i < end; i++ {
		rows = append(rows, build(i, entries[i]))
	}
	return rows
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func RenderRows(rows []RenderableRow, innerH int, emptyLine string, accent color.Color) []string {
	lines := make([]string, 0, innerH)
	if len(rows) == 0 {
		lines = append(lines, emptyLine)
	} else {
		for _, row := range rows {
			mark := " "
			if row.Selected {
				mark = lipgloss.NewStyle().Foreground(accent).Render("▌")
			}
			indent := strings.Repeat("  ", row.Depth)
			metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(row.Color))
			nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(row.Color))
			if row.Faint {
				metaStyle = metaStyle.Faint(true)
				nameStyle = nameStyle.Faint(true)
			}
			meta := metaStyle.Render(row.MetaRaw)
			name := nameStyle.Render(row.NameRaw)
			sep := " "
			if strings.TrimSpace(row.MetaRaw) == "" {
				sep = ""
			}
			line := fmt.Sprintf("%s%s%s%s%s", mark, indent, meta, sep, name)
			if row.Selected && !row.Faint {
				line = lipgloss.NewStyle().Bold(true).Render(line)
			}
			lines = append(lines, line)
		}
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	return lines
}
