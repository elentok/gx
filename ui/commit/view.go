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

// badgeSubjectSeparator delicately separates decoration badges from the
// subject when they share a line, without a background box that would shift
// the subject's start column relative to lines with no decorations.
var badgeSubjectSeparator = ui.StyleHint.Render(" · ")

func (m Model) View() tea.View {
	if !m.ready {
		return ui.NewMainView("\n  Loading commit…")
	}
	if m.err != nil {
		return ui.NewMainView("\n  Error: " + m.err.Error())
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
	if prefix := m.keys.Prefix(); len(prefix) > 0 {
		hints := ui.ChordBindingsFromHints(m.keys.ChordHints())
		if len(hints) > 0 {
			out = ui.OverlayBottomRight(out, ui.RenderChordOverlay(prefix[0], hints), m.width, m.height)
		}
	}
	if m.amendConfirm.IsOpen {
		out = ui.OverlayCenter(out, m.amendConfirm.View(m.width), m.width, m.height)
	}
	if m.reword.IsOpen {
		out = ui.OverlayCenter(out, m.reword.View(m.width), m.width, m.height)
	}
	if m.help.IsOpen {
		out = ui.OverlayCenter(out, m.help.View(), m.width, m.height)
	}
	return ui.NewMainView(out)
}

func (m Model) headerLines() []string {
	subjectLine := commitSubjectStyle.Render(m.details.Subject) + commitMetaStyle.Render(" (by "+m.details.AuthorName+")")
	badges := decorationBadgeParts(m.details.Decorations)
	lines := badgeLinesWithTrailingSubject(badges, subjectLine, max(1, m.width-2))
	if m.bodyExpanded {
		body := m.commitMessageBody()
		if body != "" {
			lines = append(lines, "")
			lines = append(lines, strings.Split(body, "\n")...)
		}
	}
	return lines
}

// badgeLinesWithTrailingSubject packs badges onto one or more lines, then
// appends subjectLine to the last badge line if it fits; otherwise subject
// gets its own line. With no badges, subjectLine is the sole line.
func badgeLinesWithTrailingSubject(badges []string, subjectLine string, maxWidth int) []string {
	lines := packBadgeLines(badges, maxWidth)
	if len(lines) == 0 {
		return []string{subjectLine}
	}
	lastIdx := len(lines) - 1
	avail := maxWidth - ansi.StringWidth(lines[lastIdx])
	if ansi.StringWidth(subjectLine)+ansi.StringWidth(badgeSubjectSeparator) <= avail {
		lines[lastIdx] = lines[lastIdx] + badgeSubjectSeparator + subjectLine
	} else {
		lines = append(lines, subjectLine)
	}
	return lines
}

// packBadgeLines greedily packs badges onto full-width lines.
func packBadgeLines(badges []string, maxWidth int) []string {
	var lines []string
	current := ""
	currentW := 0
	for _, badge := range badges {
		badgeW := ansi.StringWidth(badge)
		if current == "" {
			current = badge
			currentW = badgeW
			continue
		}
		if currentW+1+badgeW > maxWidth {
			lines = append(lines, current)
			current = badge
			currentW = badgeW
			continue
		}
		current += " " + badge
		currentW += 1 + badgeW
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func (m Model) headerTitle() string {
	if !m.bodyExpanded && m.commitMessageBody() != "" {
		return "Commit " + commitMetaStyle.Render("(b to expand)")
	}
	return "Commit"
}

func (m Model) headerPaneTitleColor() color.Color {
	if m.isContainerFocused() && m.focusHeader {
		return ui.ColorOrange
	}
	return ui.ColorBlue
}

func (m Model) headerPaneBorderColor() color.Color {
	if m.isContainerFocused() && m.focusHeader {
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
	maxBody := max(1, available/2)
	naturalBody := max(1, len(m.headerLines())) + 2
	bodyH = min(maxBody, naturalBody)
	contentH = available - bodyH
	if contentH < 1 {
		contentH = 1
	}
	return bodyH, contentH
}

func (m Model) headerViewportRowsCount() int {
	bodyH, _ := m.layoutHeights()
	return max(1, bodyH-2)
}

func (m Model) visibleHeaderLines(viewportRows int) []string {
	all := m.headerLines()
	if len(all) == 0 {
		return []string{""}
	}
	if viewportRows < 1 {
		viewportRows = 1
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
	lines := m.visibleFileLines(width, height)
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
		bodyH := max(1, height-2)
		innerW := max(1, width-2)
		lines = m.diffModel.RenderRows(bodyH, m.focusDiff, diffview.RenderOpts{
			AccentColor: ui.ColorOrange,
			InnerWidth:  innerW,
			SearchMatch: func(displayIdx int) (bool, bool) {
				return m.diffModel.SearchMatchAt(displayIdx)
			},
			SearchQuery: m.diffModel.Search().Query(),
		})
	} else if len(m.diffModel.Data().Parsed.Lines) > 0 {
		if diffcore.HasBinaryDiff(m.diffModel.Data().Parsed) {
			lines = m.binaryDiffLines(max(1, height-2), max(1, width-2))
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
	ctx := fmt.Sprintf("Context: %d", m.currentDiffContextLines())
	return diffview.JoinDot(ctx, m.diffModel.StatusText(m.focusDiff), m.diffSearchCounterText())
}

func (m Model) diffSearchCounterText() string {
	s := m.diffModel.Search()
	if !s.HasQuery() || s.MatchesCount() == 0 || !m.focusDiff {
		return ""
	}
	cursor := s.Cursor() + 1
	total := s.MatchesCount()
	icon := "⌕"
	if m.settings.UseNerdFontIcons {
		icon = ui.Icons(true).Search
	}
	return fmt.Sprintf("%s %d/%d", icon, cursor, total)
}

func renderBadges(decorations []git.RefDecoration) string {
	return strings.Join(decorationBadgeParts(decorations), " ")
}

func decorationBadgeParts(decorations []git.RefDecoration) []string {
	if len(decorations) == 0 {
		return nil
	}
	parts := make([]string, 0, len(decorations))
	for _, decoration := range decorations {
		parts = append(parts, ui.RenderBadgeText(decoration.Name, badgeColorForDecoration(decoration)))
	}
	return parts
}

func badgeColorForDecoration(decoration git.RefDecoration) color.Color {
	switch decoration.Kind {
	case git.RefDecorationTag:
		return ui.ColorBlue
	case git.RefDecorationRemoteBranch, git.RefDecorationLocalBranch:
		if isMainOrMasterRef(decoration.Name) {
			return ui.ColorYellow
		}
		return ui.ColorMauve
	default:
		return ui.ColorSubtle
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
	left := ""
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

func (m Model) visibleFileLines(width, height int) []string {
	opts := m.filetreeRenderOpts()
	opts.Width = width - 2
	return m.fileTreeModel.RenderLines(height, opts)
}

func (m Model) requiredFilesPaneWidth(height int) int {
	required := ansi.StringWidth(" Files ")
	if w := m.fileTreeModel.RequiredWidth(height, m.filetreeRenderOpts()); w > required {
		required = w
	}
	// 2 for the frame + 1 for padding
	return required + 2 + 1
}

func (m Model) filetreeRenderOpts() filetree.RenderOpts[git.CommitFile] {
	return filetree.RenderOpts[git.CommitFile]{
		AccentColor:      ui.ColorOrange,
		Active:           m.isContainerFocused() && !m.focusDiff && !m.focusHeader,
		EmptyLine:        ui.StyleMuted.Render("no changed files"),
		UseNerdFontIcons: m.settings.UseNerdFontIcons,
		FileIcon: func(entry filetree.Entry[git.CommitFile]) string {
			return commitFileIcon(entry.Value, m.settings.UseNerdFontIcons)
		},
		FileLabel: func(entry filetree.Entry[git.CommitFile]) string {
			if entry.Value.RenameFrom != "" {
				return entry.Value.RenameFrom + " -> " + entry.Value.Path
			}
			return entry.DisplayName
		},
		MetaText: func(entry filetree.Entry[git.CommitFile]) string {
			return commitEntryMeta(entry, m.settings.UseNerdFontIcons)
		},
		RowColor: func(entry filetree.Entry[git.CommitFile]) string {
			return commitEntryColor(entry)
		},
	}
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
	if !m.isContainerFocused() || m.focusDiff || m.focusHeader {
		return ui.ColorBlue
	}
	return ui.ColorOrange
}

func (m Model) filesPaneBorderColor() color.Color {
	if !m.isContainerFocused() || m.focusDiff || m.focusHeader {
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
	if m.isContainerFocused() && m.focusDiff {
		return ui.ColorOrange
	}
	return ui.ColorBlue
}

func (m Model) diffPaneBorderColor() color.Color {
	if m.isContainerFocused() && m.focusDiff {
		return ui.ColorOrange
	}
	return ui.ColorBorder
}

func (m Model) isContainerFocused() bool {
	return !m.inactive
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
