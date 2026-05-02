package log

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	logHashStyle   = lipgloss.NewStyle().Foreground(ui.ColorBlue)
	logMetaStyle   = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	logPseudoStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true)
	logSearchStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true).Underline(true)
)

func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("\n  Loading log…")
	}
	if m.err != nil {
		return tea.NewView("\n  Error: " + m.err.Error())
	}

	body := ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       maxInt(20, m.width),
		Height:      maxInt(4, m.height-1),
		Title:       "Log",
		RightTitle:  m.startRef,
		Lines:       m.visibleLines(),
		BorderColor: ui.ColorBorder,
		TitleColor:  ui.ColorBlue,
		Background:  ui.ColorBase,
	})
	footer := m.footerView()
	out := lipgloss.JoinVertical(lipgloss.Left, body, footer)
	if m.searchMode == searchModeInput {
		overlay := m.searchOverlayView()
		y := m.settings.InputModalBottom.ResolveY(m.height, lipgloss.Height(overlay))
		out = ui.OverlayBottomCenter(out, overlay, m.width, y)
	}
	v := tea.NewView(out)
	v.AltScreen = true
	return v
}

func (m Model) visibleLines() []string {
	if len(m.rows) == 0 {
		return []string{ui.StyleMuted.Render("no commits")}
	}

	innerHeight := maxInt(1, m.height-3)
	start := m.cursor - innerHeight/2
	if start < 0 {
		start = 0
	}
	if start > len(m.rows)-innerHeight {
		start = len(m.rows) - innerHeight
	}
	if start < 0 {
		start = 0
	}
	end := start + innerHeight
	if end > len(m.rows) {
		end = len(m.rows)
	}

	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(m.rows[i], i == m.cursor, m.width-4))
	}
	return lines
}

func (m Model) renderRow(row row, selected bool, width int) string {
	line := ""
	switch row.kind {
	case rowPseudoStatus:
		line = fmt.Sprintf(
			"  %s  %s",
			logPseudoStyle.Render(m.highlightSearch("working tree")),
			ui.StyleMuted.Render(m.highlightSearch(row.detail)),
		)
	default:
		line = m.renderCommitRow(row)
		if badges := m.renderBadges(row.commit.Decorations); badges != "" {
			line += "  " + badges
		}
	}
	line = ansi.Truncate(line, maxInt(1, width), "…")
	if selected {
		lineW := ansi.StringWidth(line)
		if lineW < width {
			line += strings.Repeat(" ", width-lineW)
		}
		return ui.RenderRowHighlight(line)
	}
	return line
}

func (m Model) renderCommitRow(row row) string {
	graph := row.commit.Graph
	if graph == "" {
		graph = "*"
	}
	cols := []ui.FixedColumn{
		{Text: graph, Width: 4},
		{Text: m.highlightSearch(row.commit.Hash), Width: 8, Style: logHashStyle},
		{Text: "", Width: 2},
		{Text: ui.RelativeTimeCompact(row.commit.Date), Width: 10, Style: logMetaStyle},
		{Text: "", Width: 1},
		{Text: m.highlightSearch(row.commit.AuthorShort), Width: 4, Style: logMetaStyle},
	}
	meta := ui.RenderFixedColumns(cols)
	return meta + "  " + m.highlightSearch(row.commit.Subject)
}

func (m Model) renderBadges(decorations []git.RefDecoration) string {
	if len(decorations) == 0 {
		return ""
	}
	parts := make([]string, 0, len(decorations))
	for _, decoration := range decorations {
		parts = append(parts, ui.RenderBadge(m.highlightSearch(decoration.Name), badgeVariantForDecoration(decoration), true))
	}
	return strings.Join(parts, " ")
}

func (m Model) highlightSearch(text string) string {
	query := strings.TrimSpace(m.searchQuery)
	if query == "" {
		return text
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	if !strings.Contains(lowerText, lowerQuery) {
		return text
	}

	var out strings.Builder
	start := 0
	for start < len(text) {
		idx := strings.Index(lowerText[start:], lowerQuery)
		if idx < 0 {
			out.WriteString(text[start:])
			break
		}
		idx += start
		out.WriteString(text[start:idx])
		end := idx + len(query)
		out.WriteString(logSearchStyle.Render(text[idx:end]))
		start = end
	}
	return out.String()
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
	if left == "" && m.searchQuery != "" && len(m.searchMatch) > 0 {
		left = fmt.Sprintf("%d/%d matches", m.searchCursor+1, len(m.searchMatch))
	}
	if left == "" {
		left = "enter open commit"
	}
	right := ui.StyleHint.Render("/ search · q back · L lazygit log")
	if m.width <= 0 {
		return left + "  " + right
	}
	left = ansi.Truncate(left, m.width, "…")
	leftW := ansi.StringWidth(left)
	rightW := ansi.StringWidth(right)
	if leftW+rightW+2 >= m.width {
		return left + "  " + ansi.Truncate(right, maxInt(0, m.width-leftW-2), "")
	}
	return left + strings.Repeat(" ", m.width-leftW-rightW) + right
}

func (m Model) searchOverlayView() string {
	outerW := m.width * 80 / 100
	if outerW > 50 {
		outerW = 50
	}
	if outerW < 20 {
		outerW = 20
	}
	ti := m.searchInput
	ti.SetWidth(outerW - 4)
	rightTitle := ""
	if m.searchQuery != "" && len(m.searchMatch) == 0 {
		rightTitle = "no matches"
	} else if len(m.searchMatch) > 0 {
		rightTitle = fmt.Sprintf("%d/%d", m.searchCursor+1, len(m.searchMatch))
	}
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         "Search",
		RightTitle:    rightTitle,
		Body:          ti.View(),
		Width:         outerW,
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		TitleInBorder: true,
	})
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
