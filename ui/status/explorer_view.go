package status

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diff/diffrender"
	"github.com/elentok/gx/ui/explorer"
	"github.com/elentok/gx/ui/search"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) renderDiffPane(width, height int) string {
	sections := m.visibleDiffSections()

	if len(sections) == 0 {
		content := lipgloss.NewStyle().Foreground(ui.ColorSubtle).Render(m.diffEmptyMessage())
		return m.panelStyle(m.focus == focusDiff).
			Width(width).
			Height(height).
			Render(content)
	}

	if len(sections) == 1 {
		section := sections[0]
		return m.renderSectionPane(width, height, m.sectionTitle(section), m.sectionState(section), section)
	}
	if m.diffFullscreen {
		return m.renderSectionPane(width, height, m.sectionTitle(m.section), m.currentSection(), m.section)
	}

	topH := height / 2
	if topH < 5 {
		topH = 5
	}
	bottomH := height - topH
	if bottomH < 5 {
		bottomH = 5
		topH = height - bottomH
	}

	topSection := sections[0]
	bottomSection := sections[1]
	top := m.renderSectionPane(width, topH, m.sectionTitle(topSection), m.sectionState(topSection), topSection)
	bottom := m.renderSectionPane(width, bottomH, m.sectionTitle(bottomSection), m.sectionState(bottomSection), bottomSection)
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

func (m *Model) renderSectionPane(width, height int, title string, sec *sectionState, section diffSection) string {
	innerW := maxInt(1, width-2)
	innerH := maxInt(1, height-2)

	activeSection := m.focus == focusDiff && m.section == section

	bodyH := innerH
	if bodyH < 0 {
		bodyH = 0
	}

	active := m.activeRawLineIndex(*sec)
	accent := ui.ColorOrange
	if section == sectionStaged {
		accent = ui.ColorGreen
	}
	sec.viewport.SetHeight(maxInt(0, bodyH))
	sec.viewport.SetWidth(innerW)

	titleText := title
	if file, ok := m.selectedExplorerFile(); ok && file.RenameFrom != "" {
		titleText += " [moved: " + file.RenameFrom + " -> " + file.Path + "]"
	}
	if si := diffrender.ParseSymlinkDiffInfo(sec.data.Parsed); si.IsSymlink {
		if label := si.TitleLabel(); label != "" {
			titleText += " " + label
		}
	}
	if m.diffFullscreen {
		titleText += " [fullscreen]"
	}
	rightTitleText := ""
	if sec.viewport.TotalLineCount() > sec.viewport.VisibleLineCount() && sec.viewport.VisibleLineCount() > 0 {
		pct := int(sec.viewport.ScrollPercent()*100 + 0.5)
		rightTitleText = fmt.Sprintf("%d%%", pct)
	}

	overflowTopMark, overflowBottomMark, overflowBothMark := m.hunkOverflowMarkers()

	lines := make([]string, 0, bodyH)
	if len(sec.data.ViewLines) == 0 && diffrender.SectionHasBinaryDiff(sec.data.Parsed) {
		lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorSubtle).Render(m.binarySummaryLine()))
	}

	if len(lines) == 0 {
		rows := explorer.BuildVisibleDiffRows(explorer.VisibleDiffRowsOptions{
			Section:    sec.data,
			ViewportY:  sec.viewport.YOffset(),
			Visible:    sec.viewport.VisibleLineCount(),
			BodyHeight: bodyH,
			NavMode:    m.navMode,
			Active:     activeSection,
			ActiveRaw:  active,
		})
		for _, row := range rows {
			if row.DisplayIndex < 0 || row.DisplayIndex >= len(sec.data.ViewLines) {
				lines = append(lines, "")
				continue
			}
			displayIdx := row.DisplayIndex
			rawIdx := row.RawIndex
			mark := "  "
			if m.navMode == navLine && sec.data.VisualActive && m.visualMatchDiffDisplay(*sec, displayIdx) {
				mark = lipgloss.NewStyle().Foreground(accent).Render("▎ ")
			}
			if row.InActiveHunk && activeSection {
				mark = lipgloss.NewStyle().Foreground(accent).Render("▌ ")
			}
			if row.IsActiveRaw {
				mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render("▌ ")
			}
			if row.IsActiveChangedRaw {
				mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render("▌ ")
			}
			if row.InActiveHunk {
				if row.OverflowTop && row.OverflowBottom {
					mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(overflowBothMark)
				} else if row.OverflowTop {
					mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(overflowTopMark)
				} else if row.OverflowBottom {
					mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(overflowBottomMark)
				}
			}
			if rawIdx >= 0 && m.flashMarker(section, rawIdx, sec) {
				mark = lipgloss.NewStyle().Foreground(ui.ColorGreen).Bold(true).Render("◆ ")
			}

			indicator := "  "
			markW := ansi.StringWidth(mark)
			indicatorW := ansi.StringWidth(indicator)
			bodyW := innerW - markW - indicatorW
			if bodyW < 0 {
				bodyW = 0
			}
			rowKind := row.Kind
			body := ansi.Truncate(row.Text, bodyW, "")
			if m.renderMode == renderSideBySide {
				plain := strings.TrimSpace(ansi.Strip(body))
				if isDeltaSectionDivider(plain) {
					body = lipgloss.NewStyle().Foreground(ui.ColorDeepBg).Render(ansi.Strip(body))
				}
			}
			if matched, current := m.searchMatchDiffDisplay(section, displayIdx); matched {
				body = search.Highlight(ansi.Strip(body), m.search.Query(), current)
			}
			body += diffrender.DiffBodyPadding(rowKind, maxInt(0, bodyW-ansi.StringWidth(body)))
			lines = append(lines, mark+body+indicator)
		}
	}
	for len(lines) < bodyH {
		lines = append(lines, "")
	}
	return m.renderPanelWithBorderTitle(width, height, titleText, rightTitleText, lines, activeSection, section)
}

func (m Model) hunkOverflowMarkers() (top, bottom, both string) {
	if m.settings.UseNerdFontIcons {
		return " ", " ", "↕ "
	}
	return "↑ ", "↓ ", "↕ "
}

func (m Model) visualMatchDiffDisplay(sec sectionState, displayIdx int) bool {
	if !sec.data.VisualActive || m.navMode != navLine {
		return false
	}
	if len(sec.data.ChangedDisplay) > 0 {
		start, end := visualLineBounds(sec)
		for i := start; i <= end && i < len(sec.data.ChangedDisplay); i++ {
			if i >= 0 && sec.data.ChangedDisplay[i] == displayIdx {
				return true
			}
		}
		return false
	}
	if displayIdx < 0 || displayIdx >= len(sec.data.DisplayToRaw) {
		return false
	}
	rawIdx := sec.data.DisplayToRaw[displayIdx]
	if rawIdx < 0 {
		return false
	}
	start, end := visualLineBounds(sec)
	for i := start; i <= end && i < len(sec.data.Parsed.Changed); i++ {
		if i >= 0 && sec.data.Parsed.Changed[i].LineIndex == rawIdx {
			return true
		}
	}
	return false
}

func (m Model) activeRawLineIndex(sec sectionState) int {
	if m.navMode == navHunk {
		if sec.data.ActiveHunk >= 0 && sec.data.ActiveHunk < len(sec.data.Parsed.Hunks) {
			return sec.data.Parsed.Hunks[sec.data.ActiveHunk].StartLine
		}
		return -1
	}
	if sec.data.ActiveLine >= 0 && sec.data.ActiveLine < len(sec.data.Parsed.Changed) {
		return sec.data.Parsed.Changed[sec.data.ActiveLine].LineIndex
	}
	return -1
}

func (m *Model) syncDiffViewports() {
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	_, diffW := m.splitWidth()
	_, diffH := m.splitHeight(mainH)
	if m.diffFullscreen && m.focus == focusDiff {
		diffW = m.width
	}
	vpW := maxInt(1, diffW-4)
	wrapWidth := maxInt(1, vpW-2)
	reflowSectionLines(m.sectionState(sectionUnstaged), wrapWidth, m.wrapSoft)
	reflowSectionLines(m.sectionState(sectionStaged), wrapWidth, m.wrapSoft)

	hasUnstaged := m.sectionHasContent(sectionUnstaged)
	hasStaged := m.sectionHasContent(sectionStaged)
	if m.diffFullscreen {
		if m.section == sectionUnstaged {
			m.unstaged.viewport.SetHeight(maxInt(0, diffH-3))
			m.staged.viewport.SetHeight(0)
		} else {
			m.staged.viewport.SetHeight(maxInt(0, diffH-3))
			m.unstaged.viewport.SetHeight(0)
		}
		m.unstaged.viewport.SetWidth(vpW)
		m.staged.viewport.SetWidth(vpW)
		m.unstaged.viewport.SetContentLines(m.unstaged.data.ViewLines)
		m.staged.viewport.SetContentLines(m.staged.data.ViewLines)
		return
	}

	if hasUnstaged && hasStaged {
		topH := diffH / 2
		if topH < 5 {
			topH = 5
		}
		bottomH := diffH - topH
		if bottomH < 5 {
			bottomH = 5
			topH = diffH - bottomH
		}
		m.unstaged.viewport.SetHeight(maxInt(0, topH-3))
		m.staged.viewport.SetHeight(maxInt(0, bottomH-3))
		m.unstaged.viewport.SetWidth(vpW)
		m.staged.viewport.SetWidth(vpW)
	} else if hasUnstaged {
		m.unstaged.viewport.SetHeight(maxInt(0, diffH-3))
		m.unstaged.viewport.SetWidth(vpW)
		m.staged.viewport.SetHeight(0)
		m.staged.viewport.SetWidth(vpW)
	} else if hasStaged {
		m.staged.viewport.SetHeight(maxInt(0, diffH-3))
		m.staged.viewport.SetWidth(vpW)
		m.unstaged.viewport.SetHeight(0)
		m.unstaged.viewport.SetWidth(vpW)
	} else {
		m.unstaged.viewport.SetHeight(0)
		m.staged.viewport.SetHeight(0)
		m.unstaged.viewport.SetWidth(vpW)
		m.staged.viewport.SetWidth(vpW)
	}
	m.unstaged.viewport.SetContentLines(m.unstaged.data.ViewLines)
	m.staged.viewport.SetContentLines(m.staged.data.ViewLines)
}

func reflowSectionLines(sec *sectionState, wrapWidth int, wrapSoft bool) {
	prevOffset := sec.viewport.YOffset()
	explorer.ReflowSectionData(&sec.data, wrapWidth, wrapSoft)
	if len(sec.data.BaseLines) == 0 {
		sec.viewport.SetContent("")
		sec.viewport.SetYOffset(0)
		return
	}
	sec.viewport.SetContentLines(sec.data.ViewLines)
	sec.viewport.SetYOffset(prevOffset)
}

func (m Model) binarySummaryLine() string {
	file, ok := m.selectedExplorerFile()
	if !ok {
		return "binary file"
	}
	prevSize, newSize, prevOK, newOK := git.BinaryFileSizes(m.worktreeRoot, file.stageFile)
	if !prevOK && !newOK {
		return "binary file"
	}
	return fmt.Sprintf("binary file (prev size: %s, new size: %s)", formatSize(prevSize, prevOK), formatSize(newSize, newOK))
}

// isWorktreeSymlink reports whether the file at relPath inside worktreeRoot is a
// symlink in the working tree. Returns false if the file does not exist or any
// error occurs (e.g. deleted files).
func isWorktreeSymlink(worktreeRoot, relPath string) bool {
	info, err := os.Lstat(filepath.Join(worktreeRoot, filepath.FromSlash(relPath)))
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

func formatSize(size int64, ok bool) string {
	if !ok {
		return "n/a"
	}
	if size < 0 {
		size = 0
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v := float64(size)
	idx := 0
	for v >= 1024 && idx < len(units)-1 {
		v /= 1024
		idx++
	}
	if idx == 0 {
		return fmt.Sprintf("%d %s", size, units[idx])
	}
	if v >= 10 {
		return fmt.Sprintf("%.0f %s", v, units[idx])
	}
	return fmt.Sprintf("%.1f %s", v, units[idx])
}
