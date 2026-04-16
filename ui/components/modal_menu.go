package components

import (
	"image/color"
	"strings"

	"gx/ui"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	MenuHint       = ui.RenderInlineBindings(menuNavBinding(), menuSelectBinding(), menuCancelBinding())
	MenuHintNumber = ui.RenderInlineBindings(menuNavBinding(), menuQuickSelectBinding(), menuSelectBinding(), menuAbortBinding())
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

func menuNavBinding() key.Binding {
	return key.NewBinding(key.WithHelp("j/k or ↑/↓", "navigate"))
}

func menuQuickSelectBinding() key.Binding {
	return key.NewBinding(key.WithHelp("1-9", "quick select"))
}

func menuSelectBinding() key.Binding {
	return key.NewBinding(key.WithHelp("enter", "select"))
}

func menuCancelBinding() key.Binding {
	return key.NewBinding(key.WithHelp("esc", "cancel"))
}

func menuAbortBinding() key.Binding {
	return key.NewBinding(key.WithHelp("esc", "abort"))
}

func RenderMenuList(state MenuState, subtleColor, selectedColor color.Color) string {
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
	return strings.Join(menuLines, "\n")
}

func RenderMenuModal(title, prompt string, state MenuState, hint string, borderColor, titleColor, subtleColor, selectedColor color.Color, width int) string {
	if hint == "" {
		hint = MenuHint
	}
	body := prompt
	if body != "" {
		body += "\n\n"
	}
	body += RenderMenuList(state, subtleColor, selectedColor)
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
