package components

import (
	"image/color"
	"strings"

	"gx/ui"

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
	body := prompt
	if body != "" {
		body += "\n\n"
	}
	body += strings.Join(menuLines, "\n")
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:       title,
		Body:        body,
		Hint:        hint,
		Width:       width,
		BorderColor: borderColor,
		TitleColor:  titleColor,
		HintColor:   subtleColor,
	})
}
