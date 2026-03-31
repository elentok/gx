package components

import (
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type MenuItem struct {
	Label  string
	Value  string
	Detail string
}

type MenuState struct {
	Items  []MenuItem
	Cursor int
}

func UpdateMenu(msg tea.KeyPressMsg, state MenuState) (MenuState, bool, bool, bool) {
	if len(state.Items) == 0 {
		return state, false, false, false
	}
	if state.Cursor < 0 {
		state.Cursor = 0
	}
	if state.Cursor >= len(state.Items) {
		state.Cursor = len(state.Items) - 1
	}

	switch msg.String() {
	case "j", "down":
		if state.Cursor < len(state.Items)-1 {
			state.Cursor++
		}
		return state, false, false, true
	case "k", "up":
		if state.Cursor > 0 {
			state.Cursor--
		}
		return state, false, false, true
	case "enter":
		return state, true, true, true
	case "esc", "q":
		return state, true, false, true
	default:
		return state, false, false, false
	}
}

func RenderMenuModal(title, prompt string, state MenuState, hint string, borderColor, titleColor, subtleColor, selectedColor color.Color, width int) string {
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)
	if width > 0 {
		borderStyle = borderStyle.Width(width)
	}

	head := lipgloss.NewStyle().Foreground(titleColor).Bold(true).Render(title)
	selected := lipgloss.NewStyle().Foreground(selectedColor).Bold(true)
	normal := lipgloss.NewStyle().Foreground(subtleColor)

	menuLines := make([]string, 0, len(state.Items))
	for i, it := range state.Items {
		line := "  " + it.Label
		if strings.TrimSpace(it.Detail) != "" {
			line += "  " + normal.Render(it.Detail)
		}
		if i == state.Cursor {
			line = selected.Render("▸ " + it.Label)
			if strings.TrimSpace(it.Detail) != "" {
				line += "  " + normal.Render(it.Detail)
			}
		}
		menuLines = append(menuLines, line)
	}

	if hint == "" {
		hint = "j/k or ↑/↓ navigate · enter select · esc cancel"
	}
	inner := lipgloss.JoinVertical(lipgloss.Left,
		head,
		"",
		prompt,
		"",
		strings.Join(menuLines, "\n"),
		"",
		normal.Render(hint),
	)
	return borderStyle.Render(inner)
}
