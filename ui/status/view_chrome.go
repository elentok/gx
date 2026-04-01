package stage

import (
	"strings"

	"gx/git"
	"gx/ui/components"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) errorModalView() string {
	return components.RenderOutputModal(
		"Error",
		m.errorVP.View(),
		"esc / enter dismiss · j/k scroll",
		catRed,
		catRed,
		catSubtle,
		m.errorVP.Width(),
	)
}

func (m Model) helpModalView() string {
	titleStyle := lipgloss.NewStyle().Foreground(catBlue).Bold(true)
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(catBlue).
		Padding(0, 1).
		Width(m.helpVP.Width())

	hint := lipgloss.NewStyle().Foreground(catSubtle).Render("? / esc / enter dismiss · j/k scroll")
	inner := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Keyboard Help"),
		"",
		m.helpVP.View(),
		"",
		hint,
	)
	return borderStyle.Render(inner)
}

func overlayModal(bg, modal string, screenW, screenH int) string {
	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	x := (screenW - modalW) / 2
	y := (screenH - modalH) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return placeOverlay(bg, modal, x, y)
}

func placeOverlay(bg, fg string, x, y int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for i, fgLine := range fgLines {
		bgY := y + i
		if bgY < 0 || bgY >= len(bgLines) {
			continue
		}
		bgLine := bgLines[bgY]
		fgW := ansi.StringWidth(fgLine)

		left := ansi.Truncate(bgLine, x, "")
		if leftW := ansi.StringWidth(left); leftW < x {
			left += strings.Repeat(" ", x-leftW)
		}
		right := ansi.TruncateLeft(bgLine, x+fgW, "")
		bgLines[bgY] = left + fgLine + right
	}

	return strings.Join(bgLines, "\n")
}

func (m Model) panelStyle(active bool) lipgloss.Style {
	borderColor := catSubtle
	if active {
		borderColor = catOrange
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Background(catBase0)
}

func (m Model) renderPanelWithBorderTitle(width, height int, title, rightTitle string, lines []string, active bool, section diffSection) string {
	if width < 2 || height < 2 {
		return ""
	}
	innerW := width - 2
	innerH := height - 2

	borderColor := catSubtle
	titleStyle := lipgloss.NewStyle().Foreground(catBlue)
	if section == sectionStaged {
		borderColor = catGreen
		titleStyle = lipgloss.NewStyle().Foreground(catGreen)
		if active {
			titleStyle = titleStyle.Bold(true)
		}
	} else if active {
		borderColor = catOrange
		titleStyle = lipgloss.NewStyle().Foreground(catOrange).Bold(true)
	}
	border := lipgloss.NewStyle().Foreground(borderColor)

	titleSeg := titleStyle.Render(" " + title + " ")
	rightSeg := ""
	if rightTitle != "" {
		rightSeg = titleStyle.Render(" " + rightTitle + " ")
	}
	titleW := ansi.StringWidth(titleSeg)
	rightW := ansi.StringWidth(rightSeg)
	topInner := ""
	if rightW >= innerW {
		topInner = ansi.Truncate(rightSeg, innerW, "")
	} else if titleW+rightW >= innerW {
		titleSeg = ansi.Truncate(titleSeg, innerW-rightW, "")
		titleW = ansi.StringWidth(titleSeg)
		topInner = titleSeg + rightSeg
	} else if titleW >= innerW {
		topInner = ansi.Truncate(titleSeg, innerW, "")
		titleW = ansi.StringWidth(topInner)
	} else {
		topInner = titleSeg + border.Render(strings.Repeat("─", innerW-titleW-rightW)) + rightSeg
	}
	if titleW+rightW < innerW && !strings.Contains(topInner, "─") {
		topInner += border.Render(strings.Repeat("─", innerW-titleW-rightW))
	}

	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	body := make([]string, 0, innerH)
	for i := 0; i < innerH; i++ {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], innerW, "")
		}
		line = line + strings.Repeat(" ", maxInt(0, innerW-ansi.StringWidth(line)))
		body = append(body, border.Render("│")+line+ansiReset+border.Render("│"))
	}

	bottom := border.Render("╰" + strings.Repeat("─", innerW) + "╯")
	top := border.Render("╭") + topInner + border.Render("╮")
	return strings.Join(append([]string{top}, append(body, bottom)...), "\n")
}

type statusPaneIcons struct {
	folderClosed string
	folderOpen   string
	fileModified string
	fileNew      string
	fileDeleted  string
	fileRenamed  string
	partial      string
	staged       string
}

func statusPaneIconsFor(useNerdFontIcons bool) statusPaneIcons {
	if !useNerdFontIcons {
		return statusPaneIcons{
			folderClosed: "▸",
			folderOpen:   "▾",
			fileModified: "M",
			fileNew:      "N",
			fileDeleted:  "D",
			fileRenamed:  "R",
			partial:      "+",
			staged:       "✓",
		}
	}
	return statusPaneIcons{
		folderClosed: "",
		folderOpen:   "",
		fileModified: "",
		fileNew:      "",
		fileDeleted:  "",
		fileRenamed:  "󰁔",
		partial:      "",
		staged:       "",
	}
}

func statusEntryColor(entry statusEntry) string {
	if entry.Kind == statusEntryFile && isDeletedFileStatus(entry.File) {
		return "#a6adc8"
	}
	if entry.Kind == statusEntryFile && entry.File.IsRenamed() {
		return "#89b4fa"
	}
	if entry.HasStaged && entry.HasUnstaged {
		return "#fab387"
	}
	if entry.HasStaged {
		return "#a6e3a1"
	}
	return "#cdd6f4"
}

func statusEntryMeta(entry statusEntry, useNerdFontIcons bool, icons statusPaneIcons) string {
	if entry.HasStaged && entry.HasUnstaged {
		return icons.partial
	}
	if entry.HasStaged {
		return icons.staged
	}
	if useNerdFontIcons {
		return "  "
	}
	if entry.Kind == statusEntryDir {
		return "-"
	}
	return entry.File.XY()
}

func statusFileIcon(file git.StageFileStatus, icons statusPaneIcons) string {
	if isDeletedFileStatus(file) {
		return icons.fileDeleted
	}
	if file.IsRenamed() {
		return icons.fileRenamed
	}
	if file.IsUntracked() || file.IndexStatus == 'A' {
		return icons.fileNew
	}
	return icons.fileModified
}

func isDeletedFileStatus(file git.StageFileStatus) bool {
	return file.IndexStatus == 'D' || file.WorktreeCode == 'D'
}

func (m Model) flashMarker(section diffSection, rawIdx int, sec *sectionState) bool {
	if !m.flash.active || m.flash.section != section {
		return false
	}
	if m.flash.navMode == navHunk {
		if m.flash.hunk < 0 || m.flash.hunk >= len(sec.parsed.Hunks) {
			return false
		}
		h := sec.parsed.Hunks[m.flash.hunk]
		return rawIdx >= h.StartLine && rawIdx <= h.EndLine
	}
	if m.flash.line < 0 || m.flash.line >= len(sec.parsed.Changed) {
		return false
	}
	return sec.parsed.Changed[m.flash.line].LineIndex == rawIdx
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
