package commit

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diff"
	"github.com/elentok/gx/ui/explorer"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var commitMetaStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
var commitDiffMarkerStyle = lipgloss.NewStyle().Foreground(ui.ColorOrange)
var commitDiffMarkerActiveStyle = lipgloss.NewStyle().Foreground(ui.ColorOrange).Bold(true)

func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("\n  Loading commit…")
	}
	if m.err != nil {
		return tea.NewView("\n  Error: " + m.err.Error())
	}

	body := ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       maxInt(20, m.width),
		Height:      maxInt(10, minInt(m.height-1, 12)),
		Title:       "Commit",
		RightTitle:  m.ref,
		Lines:       m.headerLines(),
		BorderColor: ui.ColorBorder,
		TitleColor:  ui.ColorBlue,
		Background:  ui.ColorBase,
	})
	content := m.contentView()
	footer := m.footerView()
	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, body, content, footer))
	v.AltScreen = true
	return v
}

func (m Model) headerLines() []string {
	lines := []string{
		ui.StyleHeading.Render(m.details.Subject),
		"",
		fmt.Sprintf("%s  %s", ui.StyleTitle.Render(m.details.Hash), commitMetaStyle.Render(ui.RelativeTimeCompact(m.details.Date))),
		commitMetaStyle.Render(m.details.Date.Format("2006-01-02 15:04:05 -0700")),
		commitMetaStyle.Render(m.details.AuthorName + " · " + m.details.AuthorShort),
	}
	if badges := renderBadges(m.details.Decorations); badges != "" {
		lines = append(lines, badges)
	}
	lines = append(lines, "")
	if m.bodyExpanded {
		body := strings.TrimSpace(m.details.Body)
		if body == "" {
			lines = append(lines, ui.StyleMuted.Render("(no body)"))
		} else {
			lines = append(lines, strings.Split(body, "\n")...)
		}
	} else {
		lines = append(lines, ui.StyleMuted.Render("(body hidden; press b to expand)"))
	}
	return lines
}

func (m Model) contentView() string {
	headerH := maxInt(4, len(m.headerLines())+2)
	contentH := maxInt(5, m.height-1-headerH-1)
	if len(m.files) == 0 {
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
	leftW := maxInt(24, m.width/4)
	if leftW > m.width-40 {
		leftW = m.width - 40
	}
	if leftW < 24 {
		leftW = 24
	}
	rightW := m.width - leftW
	left := m.renderFilesPane(leftW, mainH)
	right := m.renderDiffPane(rightW, mainH)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) renderFilesPane(width, height int) string {
	lines := make([]string, 0, maxInt(1, len(m.files)))
	for i, file := range m.files {
		name := file.Path
		if file.RenameFrom != "" {
			name = file.RenameFrom + " -> " + file.Path
		}
		line := fmt.Sprintf("%s %s", file.Status, name)
		if i == m.selected {
			line = ui.RenderRowHighlight(line)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, ui.StyleMuted.Render("no changed files"))
	}
	return ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       width,
		Height:      height,
		Title:       "Files",
		BorderColor: ui.ColorBorder,
		TitleColor:  ui.ColorBlue,
		Background:  ui.ColorBase,
		Lines:       lines,
	})
}

func (m Model) renderDiffPane(width, height int) string {
	lines := []string{ui.StyleMuted.Render("no diff")}
	if len(m.section.ViewLines) > 0 {
		lines = make([]string, 0, maxInt(1, m.diffViewport.VisibleLineCount()))
		bodyH := maxInt(1, height-2)
		for i := 0; i < bodyH; i++ {
			displayIdx := m.diffViewport.YOffset() + i
			if displayIdx >= len(m.section.ViewLines) {
				lines = append(lines, "")
				continue
			}
			mark := "  "
			if m.focusDiff {
				if m.diffNavMode == explorer.NavHunk && m.section.ActiveHunk >= 0 && m.section.ActiveHunk < len(m.section.HunkDisplayRange) {
					r := m.section.HunkDisplayRange[m.section.ActiveHunk]
					if displayIdx >= r[0] && displayIdx <= r[1] {
						mark = commitDiffMarkerStyle.Render("▌ ")
					}
				}
				if m.diffNavMode == explorer.NavLine && m.section.ActiveLine >= 0 && m.section.ActiveLine < len(m.section.ChangedDisplay) && m.section.ChangedDisplay[m.section.ActiveLine] == displayIdx {
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
		BorderColor: ui.ColorBorder,
		TitleColor:  ui.ColorBlue,
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
	left := "j/k files  enter diff  b body"
	if m.focusDiff {
		left = "j/k move  a mode  / search  y yank"
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
