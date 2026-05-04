package explorer

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

type SidebarRenderableRow struct {
	Depth    int
	MetaRaw  string
	NameRaw  string
	Color    string
	Selected bool
	Faint    bool
}

func BuildVisibleSidebarRenderableRows[T any](entries []T, selected, innerH int, build func(index int, entry T) SidebarRenderableRow) []SidebarRenderableRow {
	start, end := VisibleWindow(len(entries), selected, innerH)
	rows := make([]SidebarRenderableRow, 0, sidebarMaxInt(0, end-start))
	for i := start; i < end; i++ {
		rows = append(rows, build(i, entries[i]))
	}
	return rows
}

func VisibleWindow(total, selected, bodyH int) (start, end int) {
	if total <= 0 || bodyH <= 0 {
		return 0, 0
	}
	start = selected - bodyH/2
	if start < 0 {
		start = 0
	}
	if start > total-bodyH {
		start = total - bodyH
	}
	if start < 0 {
		start = 0
	}
	end = start + bodyH
	if end > total {
		end = total
	}
	return start, end
}

func sidebarMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func RenderSidebarRows(rows []SidebarRenderableRow, innerH int, emptyLine string, accent color.Color) []string {
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
