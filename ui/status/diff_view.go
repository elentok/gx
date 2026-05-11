package status

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/diffview/diffrender"
	"github.com/elentok/gx/ui/search"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) renderDiffPane(width, height int) string {
	if height <= 0 || width <= 0 {
		return ""
	}

	expandedH, collapsedH := diffPaneHeights(height)
	if m.diff.ActiveSection == sectionStaged {
		top := m.renderSectionPane(width, collapsedH, m.sectionTitle(sectionUnstaged), m.diff.SectionModel(sectionUnstaged), sectionUnstaged)
		bottom := m.renderSectionPane(width, expandedH, m.sectionTitle(sectionStaged), m.diff.SectionModel(sectionStaged), sectionStaged)
		return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
	}

	top := m.renderSectionPane(width, expandedH, m.sectionTitle(sectionUnstaged), m.diff.SectionModel(sectionUnstaged), sectionUnstaged)
	bottom := m.renderSectionPane(width, collapsedH, m.sectionTitle(sectionStaged), m.diff.SectionModel(sectionStaged), sectionStaged)
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

func (m *Model) renderSectionPane(width, height int, title string, diffviewModel *diffview.Model, section diffSection) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	innerW := maxInt(1, width-2)
	innerH := maxInt(1, height-2)

	activeSection := m.focus == focusDiff && m.diff.ActiveSection == section
	collapsed := !activeSection && height <= collapsedDiffSectionHeight

	bodyH := innerH
	if bodyH < 0 {
		bodyH = 0
	}

	diff := diffviewModel.DataRef()
	active := diffviewModel.ActiveRawLineIndex()
	accent := ui.ColorOrange
	if section == sectionStaged {
		accent = ui.ColorGreen
	}
	diffviewModel.Viewport().SetHeight(maxInt(0, bodyH))
	diffviewModel.Viewport().SetWidth(innerW)

	titleText := m.diffSectionPaneTitle(title, section, !collapsed)
	if si := diffrender.ParseSymlinkDiffInfo(diff.Parsed); si.IsSymlink {
		if label := si.TitleLabel(); label != "" {
			titleText += " " + label
		}
	}
	if m.diff.DiffFullscreen {
		titleText += " [fullscreen]"
	}
	rightTitleText := ""
	if diffviewModel.Viewport().TotalLineCount() > diffviewModel.Viewport().VisibleLineCount() && diffviewModel.Viewport().VisibleLineCount() > 0 {
		pct := int(diffviewModel.Viewport().ScrollPercent()*100 + 0.5)
		rightTitleText = fmt.Sprintf("%d%%", pct)
	}
	if s := m.searchCounterForDiffSection(section); s != "" {
		if rightTitleText == "" {
			rightTitleText = s
		} else {
			rightTitleText += " · " + s
		}
	}

	overflowTopMark, overflowBottomMark, overflowBothMark := m.hunkOverflowMarkers()

	lines := make([]string, 0, bodyH)
	if len(diff.ViewLines) == 0 && diffcore.HasBinaryDiff(diff.Parsed) {
		lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorSubtle).Render(m.binarySummaryLine()))
	}
	if len(lines) == 0 && bodyH > 0 {
		if placeholder := m.sectionPlaceholder(section, collapsed); placeholder != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(ui.ColorSubtle).Render(placeholder))
		}
	}

	if len(lines) == 0 {
		rows := diffview.BuildVisibleDiffRows(diffview.VisibleDiffRowsOptions{
			Section:    *diff,
			ViewportY:  diffviewModel.Viewport().YOffset(),
			Visible:    diffviewModel.Viewport().VisibleLineCount(),
			BodyHeight: bodyH,
			NavMode:    m.diff.NavMode(),
			Active:     activeSection,
			ActiveRaw:  active,
		})
		for _, row := range rows {
			if row.DisplayIndex < 0 || row.DisplayIndex >= len(diff.ViewLines) {
				lines = append(lines, "")
				continue
			}
			displayIdx := row.DisplayIndex
			rawIdx := row.RawIndex
			mark := "  "
			if m.diff.NavMode() == diffview.NavModeLine && diff.VisualActive && m.visualMatchDiffDisplay(*diff, displayIdx) {
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
			if rawIdx >= 0 && m.flashMarker(section, rawIdx, diffviewModel) {
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
			if m.diff.RenderMode() == diffview.RenderModeSideBySide {
				plain := strings.TrimSpace(ansi.Strip(body))
				if isDeltaSectionDivider(plain) {
					body = lipgloss.NewStyle().Foreground(ui.ColorDeepBg).Render(ansi.Strip(body))
				}
			}
			if matched, current := m.searchMatchDiffDisplay(section, displayIdx); matched {
				body = search.Highlight(ansi.Strip(body), m.diffSearchForSection(section).Query(), current)
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

const collapsedDiffSectionHeight = 3

func diffPaneHeights(total int) (expanded, collapsed int) {
	if total <= 0 {
		return 0, 0
	}
	collapsed = collapsedDiffSectionHeight
	if total < collapsed+3 {
		collapsed = maxInt(1, total-3)
	}
	if collapsed < 0 {
		collapsed = 0
	}
	expanded = total - collapsed
	if expanded <= 0 {
		expanded = total
		collapsed = 0
	}
	return expanded, collapsed
}

func (m Model) sectionPaneTitle(title string, section diffSection) string {
	if !m.sectionHasContent(section) {
		return title + " (empty)"
	}
	return title
}

func (m Model) diffSectionPaneTitle(title string, section diffSection, expanded bool) string {
	if !expanded {
		return m.sectionPaneTitle(title, section)
	}
	file, ok := m.selectedStatusFile()
	if !ok {
		return m.sectionPaneTitle(title, section)
	}
	return title + ": " + m.diffDisplayedPath(file.stageFile)
}

func (m Model) diffDisplayedPath(file git.StageFileStatus) string {
	if file.IsRenamed() && file.RenameFrom != "" {
		return file.RenameFrom + " -> " + file.Path
	}
	return file.Path
}

func (m Model) sectionPlaceholder(section diffSection, collapsed bool) string {
	if _, ok := m.selectedStatusFile(); !ok {
		return m.diffEmptyMessage()
	}
	if !m.sectionHasContent(section) {
		if collapsed {
			return "empty"
		}
		return "No changes"
	}
	return ""
}

func (m Model) hunkOverflowMarkers() (top, bottom, both string) {
	if m.settings.UseNerdFontIcons {
		return " ", " ", "↕ "
	}
	return "↑ ", "↓ ", "↕ "
}

func (m Model) visualMatchDiffDisplay(diffData diffview.DiffData, displayIdx int) bool {
	if !diffData.VisualActive || m.diff.NavMode() != diffview.NavModeLine {
		return false
	}
	if len(diffData.ChangedDisplay) > 0 {
		start, end := diffData.VisualLineBounds()
		for i := start; i <= end && i < len(diffData.ChangedDisplay); i++ {
			if i >= 0 && diffData.ChangedDisplay[i] == displayIdx {
				return true
			}
		}
		return false
	}
	if displayIdx < 0 || displayIdx >= len(diffData.DisplayToRaw) {
		return false
	}
	rawIdx := diffData.DisplayToRaw[displayIdx]
	if rawIdx < 0 {
		return false
	}
	start, end := diffData.VisualLineBounds()
	for i := start; i <= end && i < len(diffData.Parsed.Changed); i++ {
		if i >= 0 && diffData.Parsed.Changed[i].LineIndex == rawIdx {
			return true
		}
	}
	return false
}

func (m *Model) syncDiffViewports() {
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	_, diffW := m.splitWidth()
	_, diffH := m.splitHeight(mainH)
	if m.diff.DiffFullscreen && m.focus == focusDiff {
		diffW = m.width
	}
	vpW := maxInt(1, diffW-4)
	wrapWidth := maxInt(1, vpW-2)
	reflowSectionLines(m.diff.SectionModel(sectionUnstaged), wrapWidth, m.diff.Wrap())
	reflowSectionLines(m.diff.SectionModel(sectionStaged), wrapWidth, m.diff.Wrap())

	expandedH, collapsedH := diffPaneHeights(diffH)
	m.diff.Unstaged.Viewport().SetWidth(vpW)
	m.diff.Staged.Viewport().SetWidth(vpW)
	if m.diff.ActiveSection == sectionStaged {
		m.diff.Unstaged.Viewport().SetHeight(maxInt(0, collapsedH-3))
		m.diff.Staged.Viewport().SetHeight(maxInt(0, expandedH-3))
	} else {
		m.diff.Unstaged.Viewport().SetHeight(maxInt(0, expandedH-3))
		m.diff.Staged.Viewport().SetHeight(maxInt(0, collapsedH-3))
	}
	m.diff.Unstaged.Viewport().SetContentLines(m.diff.Unstaged.Data().ViewLines)
	m.diff.Staged.Viewport().SetContentLines(m.diff.Staged.Data().ViewLines)
}

func reflowSectionLines(diffviewModel *diffview.Model, wrapWidth int, wrap bool) {
	diffviewModel.EnableWrap(wrap)
	diffviewModel.Reflow(wrapWidth)
}

func (m Model) binarySummaryLine() string {
	file, ok := m.selectedStatusFile()
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
