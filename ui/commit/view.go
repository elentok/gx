package commit

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diff"
	"github.com/elentok/gx/ui/explorer"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var commitMetaStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
var commitSubjectStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true)
var commitDiffMarkerStyle = lipgloss.NewStyle().Foreground(ui.ColorOrange)
var commitDiffMarkerActiveStyle = lipgloss.NewStyle().Foreground(ui.ColorOrange).Bold(true)

func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("\n  Loading commit…")
	}
	if m.err != nil {
		return tea.NewView("\n  Error: " + m.err.Error())
	}

	bodyH, contentH := m.layoutHeights()
	body := ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       maxInt(20, m.width),
		Height:      bodyH,
		Title:       "Commit",
		RightTitle:  m.headerRightTitle(),
		Lines:       m.headerLines(),
		BorderColor: ui.ColorBorder,
		TitleColor:  ui.ColorBlue,
		Background:  ui.ColorBase,
	})
	content := m.contentView(contentH)
	footer := m.footerView()
	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, body, content, footer))
	v.AltScreen = true
	return v
}

func (m Model) headerLines() []string {
	lines := []string{
		commitSubjectStyle.Render(m.details.Subject) + commitMetaStyle.Render(" (by "+m.details.AuthorName+")"),
	}
	if m.bodyExpanded {
		body := m.commitMessageBody()
		if body != "" {
			lines = append(lines, "")
			lines = append(lines, strings.Split(body, "\n")...)
		}
	} else {
		lines = append(lines, "")
		lines = append(lines, ui.StyleMuted.Render("(body hidden; press b to expand)"))
	}
	return lines
}

func (m Model) commitMessageBody() string {
	body := strings.TrimSpace(m.details.Body)
	if body == "" {
		return ""
	}
	subject := strings.TrimSpace(m.details.Subject)
	if subject == "" {
		return body
	}
	lines := strings.Split(body, "\n")
	if len(lines) == 0 {
		return ""
	}
	if strings.TrimSpace(lines[0]) == subject {
		lines = lines[1:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (m Model) headerRightTitle() string {
	date := m.details.Date.Format("2006-01-02 15:04")
	rel := ui.RelativeTimeCompact(m.details.Date)
	return fmt.Sprintf(
		"%s %s %s",
		ui.StyleTitle.Render(m.details.Hash),
		commitMetaStyle.Render(date),
		commitMetaStyle.Render("("+rel+")"),
	)
}

func (m Model) contentView(contentH int) string {
	if len(m.fileEntries) == 0 {
		return ui.RenderPanelFrame(ui.PanelFrameOptions{
			Width:       maxInt(20, m.width),
			Height:      contentH,
			Title:       "Changes",
			BorderColor: ui.ColorBorder,
			TitleColor:  ui.ColorBlue,
			Background:  ui.ColorBase,
			Lines:       []string{ui.StyleMuted.Render("no changed files")},
		})
	}

	mainH := contentH
	if m.width < 90 {
		filesH := maxInt(5, mainH/3)
		diffH := maxInt(5, mainH-filesH)
		files := m.renderFilesPane(m.width, filesH)
		diff := m.renderDiffPane(m.width, diffH)
		return lipgloss.JoinVertical(lipgloss.Left, files, diff)
	}
	leftW := m.filesPaneWidth(mainH)
	rightW := m.width - leftW
	left := m.renderFilesPane(leftW, mainH)
	right := m.renderDiffPane(rightW, mainH)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) layoutHeights() (bodyH, contentH int) {
	available := maxInt(2, m.height-1) // reserve one line for footer
	naturalBody := maxInt(3, len(m.headerLines())+2)
	bodyH = minInt(12, naturalBody)
	if bodyH > available-1 {
		bodyH = maxInt(1, available-1)
	}
	contentH = available - bodyH
	if contentH < 1 {
		contentH = 1
	}
	return bodyH, contentH
}

func (m Model) renderFilesPane(width, height int) string {
	lines := m.visibleFileLines(height)
	if len(lines) == 0 {
		lines = append(lines, ui.StyleMuted.Render("no changed files"))
	}
	return ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       width,
		Height:      height,
		Title:       "Files",
		RightTitle:  m.filesPaneRightTitle(),
		BorderColor: m.filesPaneBorderColor(),
		TitleColor:  m.filesPaneTitleColor(),
		Background:  ui.ColorBase,
		Lines:       lines,
	})
}

func (m Model) renderDiffPane(width, height int) string {
	lines := []string{ui.StyleMuted.Render("no diff")}
	if len(m.section.ViewLines) > 0 {
		lines = make([]string, 0, maxInt(1, m.diffViewport.VisibleLineCount()))
		bodyH := maxInt(1, height-2)
		active := m.activeRawLineIndex()
		hunkStart, hunkEnd := -1, -1
		if m.diffNavMode == explorer.NavHunk && m.section.ActiveHunk >= 0 && m.section.ActiveHunk < len(m.section.Parsed.Hunks) {
			hunkStart = m.section.Parsed.Hunks[m.section.ActiveHunk].StartLine
			hunkEnd = m.section.Parsed.Hunks[m.section.ActiveHunk].EndLine
		}
		for i := 0; i < bodyH; i++ {
			displayIdx := m.diffViewport.YOffset() + i
			if displayIdx >= len(m.section.ViewLines) {
				lines = append(lines, "")
				continue
			}
			rawIdx := -1
			if displayIdx >= 0 && displayIdx < len(m.section.DisplayToRaw) {
				rawIdx = m.section.DisplayToRaw[displayIdx]
			}
			mark := "  "
			if m.focusDiff {
				inActiveHunk := false
				if m.diffNavMode == explorer.NavHunk {
					if m.section.ActiveHunk >= 0 && m.section.ActiveHunk < len(m.section.HunkDisplayRange) {
						r := m.section.HunkDisplayRange[m.section.ActiveHunk]
						inActiveHunk = displayIdx >= r[0] && displayIdx <= r[1]
					} else {
						inActiveHunk = rawIdx >= 0 && rawIdx >= hunkStart && rawIdx <= hunkEnd
					}
				}
				if inActiveHunk {
					mark = commitDiffMarkerStyle.Render("▌ ")
				}
				if rawIdx >= 0 && rawIdx == active {
					mark = commitDiffMarkerActiveStyle.Render("▌ ")
				}
				if rawIdx < 0 && m.diffNavMode == explorer.NavLine && m.section.ActiveLine >= 0 && m.section.ActiveLine < len(m.section.ChangedDisplay) && m.section.ChangedDisplay[m.section.ActiveLine] == displayIdx {
					mark = commitDiffMarkerActiveStyle.Render("▌ ")
				}
			}
			body := m.section.ViewLines[displayIdx]
			if matched, current := m.searchMatchDiffDisplay(displayIdx); matched {
				body = highlightMatchText(body, m.searchQuery, current)
			}
			lines = append(lines, mark+body)
		}
	} else if len(m.section.Parsed.Lines) > 0 {
		if diff.HasBinaryDiff(m.section.Parsed) {
			lines = []string{ui.StyleMuted.Render("binary file")}
		} else {
			lines = []string{ui.StyleMuted.Render("no diff")}
		}
	}
	return ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       width,
		Height:      height,
		Title:       "Diff",
		RightTitle:  m.diffTitle(),
		BorderColor: m.diffPaneBorderColor(),
		TitleColor:  m.diffPaneTitleColor(),
		Background:  ui.ColorBase,
		Lines:       lines,
	})
}

func (m Model) diffTitle() string {
	if !m.focusDiff {
		return ""
	}
	if m.diffNavMode == explorer.NavLine {
		return "line"
	}
	return "hunk"
}

func renderBadges(decorations []git.RefDecoration) string {
	if len(decorations) == 0 {
		return ""
	}
	parts := make([]string, 0, len(decorations))
	for _, decoration := range decorations {
		parts = append(parts, ui.RenderBadge(decoration.Name, badgeVariantForDecoration(decoration), true))
	}
	return strings.Join(parts, " ")
}

func badgeVariantForDecoration(decoration git.RefDecoration) ui.BadgeVariant {
	switch decoration.Kind {
	case git.RefDecorationTag:
		return ui.BadgeVariantBlue
	case git.RefDecorationRemoteBranch, git.RefDecorationLocalBranch:
		if isMainOrMasterRef(decoration.Name) {
			return ui.BadgeVariantYellow
		}
		return ui.BadgeVariantMauve
	default:
		return ui.BadgeVariantSurface
	}
}

func isMainOrMasterRef(name string) bool {
	name = strings.TrimSpace(name)
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return name == "main" || name == "master"
}

func (m Model) footerView() string {
	if m.statusMsg != "" {
		return ui.StyleHint.Render(m.statusMsg)
	}
	if m.searchMode == searchModeInput {
		return m.searchFooterText()
	}
	left := "j/k move  h/l tree  tab pane  ,/. commits"
	if m.focusDiff {
		left = "j/k move  a mode  tab pane  ,/. files  / search"
	}
	right := ui.StyleHint.Render("gw worktrees · gl log · gs status · q back")
	if m.width <= 0 {
		return left + "  " + right
	}
	leftW := len([]rune(left))
	rightW := len([]rune(right))
	if leftW+rightW >= m.width {
		return left + "  " + right
	}
	return left + strings.Repeat(" ", m.width-leftW-rightW) + right
}

func (m Model) visibleFileLines(height int) []string {
	innerH := maxInt(1, height-2)
	icons := ui.Icons(m.settings.UseNerdFontIcons)
	rows := explorer.BuildVisibleSidebarRenderableRows(m.fileEntries, m.selected, innerH, func(i int, entry commitFileEntry) explorer.SidebarRenderableRow {
		statusColor := commitEntryColor(entry)
		name := entry.DisplayName
		if entry.Kind == commitFileEntryDir {
			symbol := icons.FolderOpen
			if !entry.Expanded {
				symbol = icons.FolderClosed
			}
			name = symbol + " " + name + "/"
		} else {
			if entry.File.RenameFrom != "" {
				name = entry.File.RenameFrom + " -> " + entry.File.Path
			}
			name = commitFileIcon(entry.File, m.settings.UseNerdFontIcons) + " " + name
		}
		if matched, current := m.searchMatchSidebarIndex(i); matched {
			name = highlightMatchText(name, m.searchQuery, current)
		}
		return explorer.SidebarRenderableRow{
			Depth:    entry.Depth,
			MetaRaw:  commitEntryMeta(entry, m.settings.UseNerdFontIcons),
			NameRaw:  name,
			Color:    statusColor,
			Selected: i == m.selected,
		}
	})
	return explorer.RenderSidebarRows(rows, innerH, ui.StyleMuted.Render("no changed files"), ui.ColorOrange)
}

func (m Model) requiredFilesPaneWidth(height int) int {
	required := ansi.StringWidth(" Files ")
	for _, line := range m.visibleFileLines(height) {
		if w := ansi.StringWidth(line); w > required {
			required = w
		}
	}
	return required + 2
}

func (m Model) filesPaneWidth(height int) int {
	width := m.requiredFilesPaneWidth(height)
	maxWidth := minInt(72, int(float64(m.width)*0.45))
	if maxWidth < 24 {
		maxWidth = 24
	}
	if width < 24 {
		width = 24
	}
	if width > maxWidth {
		width = maxWidth
	}
	if m.width-width < 40 {
		width = m.width - 40
	}
	if width < 24 {
		width = 24
	}
	return width
}

func (m Model) filesPaneTitleColor() color.Color {
	if m.focusDiff {
		return ui.ColorBlue
	}
	return ui.ColorOrange
}

func (m Model) filesPaneBorderColor() color.Color {
	if m.focusDiff {
		return ui.ColorBorder
	}
	return ui.ColorOrange
}

func (m Model) filesPaneRightTitle() string {
	if m.focusDiff {
		return ""
	}
	if len(m.fileEntries) == 0 {
		return ""
	}
	return "tree"
}

func (m Model) diffPaneTitleColor() color.Color {
	if m.focusDiff {
		return ui.ColorOrange
	}
	return ui.ColorBlue
}

func (m Model) diffPaneBorderColor() color.Color {
	if m.focusDiff {
		return ui.ColorOrange
	}
	return ui.ColorBorder
}

func commitEntryMeta(entry commitFileEntry, useNerdFontIcons bool) string {
	if useNerdFontIcons {
		return "  "
	}
	if entry.Kind == commitFileEntryDir {
		return "-"
	}
	status := strings.TrimSpace(entry.File.Status)
	if status == "" {
		return "  "
	}
	if len(status) > 2 {
		status = status[:2]
	}
	return status
}

func commitEntryColor(entry commitFileEntry) string {
	if entry.Kind == commitFileEntryDir {
		return "#cdd6f4"
	}
	switch {
	case strings.HasPrefix(entry.File.Status, "D"):
		return "#a6adc8"
	case strings.HasPrefix(entry.File.Status, "R"), strings.HasPrefix(entry.File.Status, "C"):
		return "#89b4fa"
	case strings.HasPrefix(entry.File.Status, "A"):
		return "#a6e3a1"
	default:
		return "#cdd6f4"
	}
}

func commitFileIcon(file git.CommitFile, useNerdFontIcons bool) string {
	icons := ui.Icons(useNerdFontIcons)
	switch {
	case strings.HasPrefix(file.Status, "D"):
		return icons.FileDeleted
	case strings.HasPrefix(file.Status, "R"), strings.HasPrefix(file.Status, "C"):
		return icons.FileRenamed
	case strings.HasPrefix(file.Status, "A"):
		return icons.FileAdded
	default:
		return icons.FileModified
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
