package commit

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var commitMetaStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle)

func (m Model) View() tea.View {
	if !m.ready {
		return tea.NewView("\n  Loading commit…")
	}
	if m.err != nil {
		return tea.NewView("\n  Error: " + m.err.Error())
	}

	body := ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width:       maxInt(20, m.width),
		Height:      maxInt(4, m.height-1),
		Title:       "Commit",
		RightTitle:  m.ref,
		Lines:       m.visibleLines(),
		BorderColor: ui.ColorBorder,
		TitleColor:  ui.ColorBlue,
		Background:  ui.ColorBase,
	})
	footer := m.footerView()
	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, body, footer))
	v.AltScreen = true
	return v
}

func (m Model) visibleLines() []string {
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
	left := "b toggle body"
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
