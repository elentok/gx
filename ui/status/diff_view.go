package status

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/diffview/diffrender"
	"github.com/elentok/gx/ui/status/diffarea"

	"charm.land/lipgloss/v2"
)

func (m *Model) renderDiffPane(width, height int) string {
	if height <= 0 || width <= 0 {
		return ""
	}

	expandedH, collapsedH := diffPaneHeights(height)
	if m.diffarea.ActiveSection == diffarea.SectionStaged {
		top := m.renderSectionPane(width, collapsedH, diffarea.SectionUnstaged)
		bottom := m.renderSectionPane(width, expandedH, diffarea.SectionStaged)
		return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
	}

	top := m.renderSectionPane(width, expandedH, diffarea.SectionUnstaged)
	bottom := m.renderSectionPane(width, collapsedH, diffarea.SectionStaged)
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

func (m *Model) renderSectionPane(width, height int, section diffarea.Section) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	innerW := maxInt(1, width-2)
	innerH := maxInt(1, height-2)

	activeSection := m.focus == focusDiff && m.diffarea.ActiveSection == section
	collapsed := !activeSection && height <= collapsedDiffSectionHeight

	bodyH := innerH
	if bodyH < 0 {
		bodyH = 0
	}

	title := m.sectionTitle(section)
	diffviewModel := m.diffarea.SectionModel(section)
	diff := diffviewModel.DataRef()
	accent := ui.ColorOrange
	if section == diffarea.SectionStaged {
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
	if m.diffarea.Fullscreen {
		titleText += " [fullscreen]"
	}
	rightTitleText := ""
	if !collapsed {
		rightTitleText = fmt.Sprintf("Context: %d", m.currentDiffContextLines())
	}
	if diffviewModel.Viewport().TotalLineCount() > diffviewModel.Viewport().VisibleLineCount() && diffviewModel.Viewport().VisibleLineCount() > 0 {
		pct := int(diffviewModel.Viewport().ScrollPercent()*100 + 0.5)
		if rightTitleText == "" {
			rightTitleText = fmt.Sprintf("%d%%", pct)
		} else {
			rightTitleText += fmt.Sprintf(" · %d%%", pct)
		}
	}
	if s := m.searchCounterForDiffSection(section); s != "" {
		if rightTitleText == "" {
			rightTitleText = s
		} else {
			rightTitleText += " · " + s
		}
	}

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
		lines = diffviewModel.RenderRows(bodyH, activeSection, diffview.RenderOpts{
			AccentColor: accent,
			InnerWidth:  innerW,
			SearchMatch: func(displayIdx int) (bool, bool) {
				return m.searchMatchDiffDisplay(section, displayIdx)
			},
			SearchQuery: m.diffSearchForSection(section).Query(),
		})
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

func (m Model) sectionPaneTitle(title string, section diffarea.Section) string {
	if !m.sectionHasContent(section) {
		return title + " (empty)"
	}
	return title
}

func (m Model) diffSectionPaneTitle(title string, section diffarea.Section, expanded bool) string {
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

func (m Model) sectionPlaceholder(section diffarea.Section, collapsed bool) string {
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
func (m *Model) syncDiffViewports() {
	mainH := m.height - 1
	if mainH < 4 {
		mainH = 4
	}
	_, diffW := m.splitWidth()
	filetreeH, diffH := m.splitHeight(mainH)
	if m.diffarea.Fullscreen && m.focus == focusDiff {
		diffW = m.width
	}
	vpW := maxInt(1, diffW-4)
	wrapWidth := maxInt(1, vpW-2)
	reflowSectionLines(m.diffarea.SectionModel(diffarea.SectionUnstaged), wrapWidth, m.diffarea.Wrap())
	reflowSectionLines(m.diffarea.SectionModel(diffarea.SectionStaged), wrapWidth, m.diffarea.Wrap())

	expandedH, collapsedH := diffPaneHeights(diffH)
	m.diffarea.SyncViewports(vpW, expandedH, collapsedH)
	m.fileTreeModel.SetVisibleHeight(maxInt(1, filetreeH-2))
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
