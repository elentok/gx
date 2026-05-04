package status

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diff"
	"github.com/elentok/gx/ui/explorer"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) renderDiffPane(width, height int) string {
	sections := m.visibleDiffSections()

	if len(sections) == 0 {
		content := lipgloss.NewStyle().Foreground(catSubtle).Render(m.diffEmptyMessage())
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
	accent := catOrange
	if section == sectionStaged {
		accent = catGreen
	}
	hunkStart, hunkEnd := -1, -1
	if m.navMode == navHunk && sec.activeHunk >= 0 && sec.activeHunk < len(sec.parsed.Hunks) {
		hunkStart = sec.parsed.Hunks[sec.activeHunk].StartLine
		hunkEnd = sec.parsed.Hunks[sec.activeHunk].EndLine
	}
	sec.viewport.SetHeight(maxInt(0, bodyH))
	sec.viewport.SetWidth(innerW)

	titleText := title
	if file, ok := m.selectedExplorerFile(); ok && file.RenameFrom != "" {
		titleText += " [moved: " + file.RenameFrom + " -> " + file.Path + "]"
	}
	if si := diff.ParseSymlinkDiffInfo(sec.parsed); si.IsSymlink {
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

	overflowTopDisplay := -1
	overflowBottomDisplay := -1
	if m.navMode == navHunk && activeSection && sec.activeHunk >= 0 {
		if start, end, ok := hunkDisplayBounds(*sec, sec.activeHunk); ok && sec.viewport.VisibleLineCount() > 0 {
			vpTop := sec.viewport.YOffset()
			vpBottom := vpTop + sec.viewport.VisibleLineCount() - 1
			if start < vpTop {
				overflowTopDisplay = vpTop
			}
			if end > vpBottom {
				overflowBottomDisplay = vpBottom
			}
		}
	}
	overflowTopMark, overflowBottomMark, overflowBothMark := m.hunkOverflowMarkers()

	lines := make([]string, 0, bodyH)
	if len(sec.viewLines) == 0 && diff.SectionHasBinaryDiff(sec.parsed) {
		lines = append(lines, lipgloss.NewStyle().Foreground(catSubtle).Render(m.binarySummaryLine()))
	}

	if len(lines) == 0 {
		for i := 0; i < bodyH; i++ {
			displayIdx := sec.viewport.YOffset() + i
			if displayIdx >= len(sec.viewLines) {
				lines = append(lines, "")
				continue
			}
			rawIdx := -1
			if displayIdx >= 0 && displayIdx < len(sec.displayToRaw) {
				rawIdx = sec.displayToRaw[displayIdx]
			}
			mark := "  "
			if m.navMode == navLine && sec.visualActive && m.visualMatchDiffDisplay(*sec, displayIdx) {
				mark = lipgloss.NewStyle().Foreground(accent).Render("▎ ")
			}
			inActiveHunk := false
			if m.navMode == navHunk {
				if len(sec.hunkDisplayRange) > 0 && sec.activeHunk >= 0 && sec.activeHunk < len(sec.hunkDisplayRange) {
					r := sec.hunkDisplayRange[sec.activeHunk]
					inActiveHunk = displayIdx >= r[0] && displayIdx <= r[1]
				} else {
					inActiveHunk = rawIdx >= 0 && rawIdx >= hunkStart && rawIdx <= hunkEnd
				}
			}
			if inActiveHunk && activeSection {
				mark = lipgloss.NewStyle().Foreground(accent).Render("▌ ")
			}
			if rawIdx >= 0 && rawIdx == active && activeSection {
				mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render("▌ ")
			}
			if rawIdx < 0 && m.navMode == navLine && activeSection && sec.activeLine >= 0 && sec.activeLine < len(sec.changedDisplay) && sec.changedDisplay[sec.activeLine] == displayIdx {
				mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render("▌ ")
			}
			if inActiveHunk {
				if displayIdx == overflowTopDisplay && displayIdx == overflowBottomDisplay {
					mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(overflowBothMark)
				} else if displayIdx == overflowTopDisplay {
					mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(overflowTopMark)
				} else if displayIdx == overflowBottomDisplay {
					mark = lipgloss.NewStyle().Foreground(accent).Bold(true).Render(overflowBottomMark)
				}
			}
			if rawIdx >= 0 && m.flashMarker(section, rawIdx, sec) {
				mark = lipgloss.NewStyle().Foreground(catGreen).Bold(true).Render("◆ ")
			}

			indicator := "  "
			markW := ansi.StringWidth(mark)
			indicatorW := ansi.StringWidth(indicator)
			bodyW := innerW - markW - indicatorW
			if bodyW < 0 {
				bodyW = 0
			}
			rowKind := diffRowPlain
			if displayIdx >= 0 && displayIdx < len(sec.viewLineKinds) {
				rowKind = sec.viewLineKinds[displayIdx]
			}
			body := ansi.Truncate(sec.viewLines[displayIdx], bodyW, "")
			if m.renderMode == renderSideBySide {
				plain := strings.TrimSpace(ansi.Strip(body))
				if isDeltaSectionDivider(plain) {
					body = lipgloss.NewStyle().Foreground(catDeepBg).Render(ansi.Strip(body))
				}
			}
			if matched, current := m.searchMatchDiffDisplay(section, displayIdx); matched {
				body = highlightMatchText(ansi.Strip(body), m.searchQuery, current)
			}
			body += diff.DiffBodyPadding(rowKind, maxInt(0, bodyW-ansi.StringWidth(body)))
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
	if !sec.visualActive || m.navMode != navLine {
		return false
	}
	if len(sec.changedDisplay) > 0 {
		start, end := visualLineBounds(sec)
		for i := start; i <= end && i < len(sec.changedDisplay); i++ {
			if i >= 0 && sec.changedDisplay[i] == displayIdx {
				return true
			}
		}
		return false
	}
	if displayIdx < 0 || displayIdx >= len(sec.displayToRaw) {
		return false
	}
	rawIdx := sec.displayToRaw[displayIdx]
	if rawIdx < 0 {
		return false
	}
	start, end := visualLineBounds(sec)
	for i := start; i <= end && i < len(sec.parsed.Changed); i++ {
		if i >= 0 && sec.parsed.Changed[i].LineIndex == rawIdx {
			return true
		}
	}
	return false
}

func (m Model) activeRawLineIndex(sec sectionState) int {
	if m.navMode == navHunk {
		if sec.activeHunk >= 0 && sec.activeHunk < len(sec.parsed.Hunks) {
			return sec.parsed.Hunks[sec.activeHunk].StartLine
		}
		return -1
	}
	if sec.activeLine >= 0 && sec.activeLine < len(sec.parsed.Changed) {
		return sec.parsed.Changed[sec.activeLine].LineIndex
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
		m.unstaged.viewport.SetContentLines(m.unstaged.viewLines)
		m.staged.viewport.SetContentLines(m.staged.viewLines)
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
	m.unstaged.viewport.SetContentLines(m.unstaged.viewLines)
	m.staged.viewport.SetContentLines(m.staged.viewLines)
}

func reflowSectionLines(sec *sectionState, wrapWidth int, wrapSoft bool) {
	prevOffset := sec.viewport.YOffset()
	data := toExplorerSectionData(*sec)
	explorer.ReflowSectionData(&data, wrapWidth, wrapSoft)
	*sec = fromExplorerSectionData(data, sec.viewport)
	if len(sec.baseLines) == 0 {
		sec.viewport.SetContent("")
		sec.viewport.SetYOffset(0)
		return
	}
	sec.viewport.SetContentLines(sec.viewLines)
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
