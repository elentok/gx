package commit

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/diffview/diffcore"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/search"
	"github.com/elentok/gx/ui/sidebar"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	FILE_TREE_MIN_WIDTH = 25
	FILE_TREE_MAX_WIDTH = 45
	DIFF_MIN_WIDTH      = 40
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
	headerRows := max(1, bodyH-2)
	body := ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       max(20, m.width),
		Height:      bodyH,
		Title:       m.headerTitle(),
		RightTitle:  m.headerRightTitle(),
		Lines:       m.visibleHeaderLines(headerRows),
		BorderColor: m.headerPaneBorderColor(),
		TitleColor:  m.headerPaneTitleColor(),
		Background:  ui.ColorBase,
	})
	content := m.contentView(contentH)
	footer := m.footerView()
	out := lipgloss.JoinVertical(lipgloss.Left, body, content, footer)
	if m.search.Mode() == search.SearchModeInput {
		overlayW := m.searchOverlayWidth()
		m.search.SetWidth(overlayW)
		overlay := m.search.View()
		y := m.height - 2 - lipgloss.Height(overlay)
		out = ui.OverlayBottomCenter(out, overlay, m.width, y)
	}
	if prefix := m.keys.Prefix(); len(prefix) > 0 {
		hints := ui.ChordBindingsFromHints(m.keys.ChordHints())
		if len(hints) > 0 {
			out = ui.OverlayBottomRight(out, ui.RenderChordOverlay(prefix[0], hints), m.width, m.height)
		}
	}
	if m.amendConfirm.IsOpen {
		out = ui.OverlayCenter(out, m.amendConfirm.View(m.width), m.width, m.height)
	}
	if m.help.IsOpen {
		out = ui.OverlayCenter(out, m.help.View(), m.width, m.height)
	}
	v := tea.NewView(out)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
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
	}
	return lines
}

func (m Model) headerTitle() string {
	if !m.bodyExpanded {
		return "Commit " + commitMetaStyle.Render("(b to expand)")
	}
	return "Commit"
}

func (m Model) headerPaneTitleColor() color.Color {
	if m.focusHeader {
		return ui.ColorOrange
	}
	return ui.ColorBlue
}

func (m Model) headerPaneBorderColor() color.Color {
	if m.focusHeader {
		return ui.ColorOrange
	}
	return ui.ColorBorder
}

func (m Model) commitMessageBody() string {
	body := normalizeCommitBody(m.details.Body)
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

func normalizeCommitBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.TrimSpace(body)
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
	if len(m.fileTreeModel.Entries()) == 0 {
		return ui.RenderPanelFrame(ui.PanelFrameOptions{
			Width:       max(20, m.width),
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
		filesH := max(5, mainH/3)
		diffH := max(5, mainH-filesH)
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
	available := max(2, m.height-1) // reserve one line for footer
	naturalBody := min(commitHeaderMaxRows, max(1, len(m.headerLines()))) + 2
	bodyH = min(12, naturalBody)
	if bodyH > available-1 {
		bodyH = max(1, available-1)
	}
	contentH = available - bodyH
	if contentH < 1 {
		contentH = 1
	}
	return bodyH, contentH
}

func (m Model) headerViewportRowsCount() int {
	rows := min(commitHeaderMaxRows, max(1, len(m.headerLines())))
	if rows < 1 {
		return 1
	}
	return rows
}

func (m Model) visibleHeaderLines(viewportRows int) []string {
	all := m.headerLines()
	if len(all) == 0 {
		return []string{""}
	}
	if viewportRows < 1 {
		viewportRows = 1
	}
	if viewportRows > commitHeaderMaxRows {
		viewportRows = commitHeaderMaxRows
	}
	if viewportRows > len(all) {
		viewportRows = len(all)
	}
	offset := m.headerOffset
	maxOffset := max(0, len(all)-viewportRows)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	out := make([]string, 0, viewportRows)
	for i := 0; i < viewportRows; i++ {
		out = append(out, all[offset+i])
	}
	if len(all) > viewportRows {
		topMark, bottomMark := "↑ ", "↓ "
		if m.settings.UseNerdFontIcons {
			topMark, bottomMark = " ", " "
		}
		markerStyle := commitMetaStyle.Bold(true)
		if offset > 0 && len(out) > 0 {
			out[0] = markerStyle.Render(topMark) + out[0]
		}
		if offset+viewportRows < len(all) && len(out) > 0 {
			out[len(out)-1] = markerStyle.Render(bottomMark) + out[len(out)-1]
		}
	}
	return out
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
	if len(m.diffModel.Data().ViewLines) > 0 {
		lines = make([]string, 0, max(1, m.diffModel.Viewport().VisibleLineCount()))
		bodyH := max(1, height-2)
		rows := m.diffModel.VisibleRows(bodyH, m.focusDiff)
		for _, row := range rows {
			if row.DisplayIndex < 0 || row.DisplayIndex >= len(m.diffModel.Data().ViewLines) {
				lines = append(lines, "")
				continue
			}
			displayIdx := row.DisplayIndex
			mark := "  "
			if m.focusDiff {
				if row.InActiveHunk {
					mark = commitDiffMarkerStyle.Render("▌ ")
				}
				if row.IsActiveRaw {
					mark = commitDiffMarkerActiveStyle.Render("▌ ")
				}
				if row.IsActiveChangedRaw {
					mark = commitDiffMarkerActiveStyle.Render("▌ ")
				}
			}
			body := row.Text
			if matched, current := m.searchMatchDiffDisplay(displayIdx); matched {
				body = highlightMatchText(body, m.search.Query(), current)
			}
			lines = append(lines, mark+body)
		}
	} else if len(m.diffModel.Data().Parsed.Lines) > 0 {
		if diffcore.HasBinaryDiff(m.diffModel.Data().Parsed) {
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
	if m.diffModel.NavMode() == diffview.NavModeLine {
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
	left := m.statusMsg
	right := ui.StyleHint.Render("? help")
	if m.width <= 0 {
		if left == "" {
			return right
		}
		return left + "  " + right
	}
	rightW := ansi.StringWidth(right)
	if left == "" {
		return strings.Repeat(" ", m.width-rightW) + right
	}
	leftW := ansi.StringWidth(left)
	if leftW+rightW+2 >= m.width {
		return ansi.Truncate(left, m.width-rightW-2, "…") + "  " + right
	}
	return left + strings.Repeat(" ", m.width-leftW-rightW) + right
}

func (m Model) visibleFileLines(height int) []string {
	innerH := max(1, height-2)
	icons := ui.Icons(m.settings.UseNerdFontIcons)
	entries := m.fileTreeModel.Entries()
	selected := m.fileTreeModel.SelectedIndex()
	rows := sidebar.BuildVisibleRenderableRows(entries, selected, innerH, func(i int, entry filetree.Entry[git.CommitFile]) sidebar.RenderableRow {
		statusColor := commitEntryColor(entry)
		name := entry.DisplayName
		if entry.Kind == filetree.EntryDir {
			symbol := icons.FolderOpen
			if !entry.Expanded {
				symbol = icons.FolderClosed
			}
			name = symbol + " " + name + "/"
		} else {
			if entry.Value.RenameFrom != "" {
				name = entry.Value.RenameFrom + " -> " + entry.Value.Path
			}
			name = commitFileIcon(entry.Value, m.settings.UseNerdFontIcons) + " " + name
		}
		if matched, current := m.searchMatchSidebarIndex(i); matched {
			name = highlightMatchText(name, m.search.Query(), current)
		}
		return sidebar.RenderableRow{
			Depth:    entry.Depth,
			MetaRaw:  commitEntryMeta(entry, m.settings.UseNerdFontIcons),
			NameRaw:  name,
			Color:    statusColor,
			Selected: i == selected,
		}
	})
	return sidebar.RenderRows(rows, innerH, ui.StyleMuted.Render("no changed files"), ui.ColorOrange)
}

func (m Model) requiredFilesPaneWidth(height int) int {
	required := ansi.StringWidth(" Files ")
	for _, line := range m.visibleFileLines(height) {
		if w := ansi.StringWidth(line); w > required {
			required = w
		}
	}
	// 2 for the frame + 1 for padding
	return required + 2 + 1
}

func (m Model) filesPaneWidth(height int) int {
	width := max(FILE_TREE_MIN_WIDTH, min(FILE_TREE_MAX_WIDTH, m.requiredFilesPaneWidth(height)))
	if m.width-width < DIFF_MIN_WIDTH {
		width = m.width - DIFF_MIN_WIDTH
	}
	if width < FILE_TREE_MIN_WIDTH {
		width = FILE_TREE_MIN_WIDTH
	}
	return width
}

func (m Model) filesPaneTitleColor() color.Color {
	if m.focusDiff || m.focusHeader {
		return ui.ColorBlue
	}
	return ui.ColorOrange
}

func (m Model) filesPaneBorderColor() color.Color {
	if m.focusDiff || m.focusHeader {
		return ui.ColorBorder
	}
	return ui.ColorOrange
}

func (m Model) filesPaneRightTitle() string {
	if m.focusDiff {
		return ""
	}
	if len(m.fileTreeModel.Entries()) == 0 {
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

func commitEntryMeta(entry filetree.Entry[git.CommitFile], useNerdFontIcons bool) string {
	if useNerdFontIcons {
		return "  "
	}
	if entry.Kind == filetree.EntryDir {
		return "-"
	}
	status := strings.TrimSpace(entry.Value.Status)
	if status == "" {
		return "  "
	}
	if len(status) > 2 {
		status = status[:2]
	}
	return status
}

func commitEntryColor(entry filetree.Entry[git.CommitFile]) string {
	if entry.Kind == filetree.EntryDir {
		return "#cdd6f4"
	}
	switch {
	case strings.HasPrefix(entry.Value.Status, "D"):
		return "#a6adc8"
	case strings.HasPrefix(entry.Value.Status, "R"), strings.HasPrefix(entry.Value.Status, "C"):
		return "#89b4fa"
	case strings.HasPrefix(entry.Value.Status, "A"):
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
