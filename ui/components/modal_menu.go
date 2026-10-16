package components

import (
	"image/color"
	"strings"

	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var MenuHint = ui.RenderInlineBindings(menuNavBinding(), menuFilterBinding(), menuSelectBinding(), menuCancelBinding())

type MenuItem struct {
	Label  string
	Value  string
	Detail string
}

// MenuState is a modal menu. Cursor indexes Visible(), not Items.
type MenuState struct {
	Items  []MenuItem
	Cursor int
	Filter string
}

// Visible returns the items matching Filter, in their original order.
func (s MenuState) Visible() []MenuItem {
	if s.Filter == "" {
		return s.Items
	}
	out := make([]MenuItem, 0, len(s.Items))
	for _, it := range s.Items {
		if ui.FuzzyMatch(s.Filter, it.Label, it.Detail) {
			out = append(out, it)
		}
	}
	return out
}

// Selected returns the item under the cursor, or false when the cursor is not
// on a visible item.
func (s MenuState) Selected() (MenuItem, bool) {
	visible := s.Visible()
	if s.Cursor < 0 || s.Cursor >= len(visible) {
		return MenuItem{}, false
	}
	return visible[s.Cursor], true
}

func clampCursor(cursor, n int) int {
	return max(0, min(cursor, n-1))
}

// UpdateMenu returns (state, decided, accepted, handled). Every printable key
// is filter text, so a menu has no letter or digit shortcuts.
func UpdateMenu(msg tea.KeyPressMsg, state MenuState) (MenuState, bool, bool, bool) {
	if len(state.Items) == 0 {
		return state, false, false, false
	}
	n := len(state.Visible())
	state.Cursor = clampCursor(state.Cursor, n)

	switch msg.String() {
	case "ctrl+n", "down":
		if state.Cursor < n-1 {
			state.Cursor++
		}
		return state, false, false, true
	case "ctrl+p", "up":
		if state.Cursor > 0 {
			state.Cursor--
		}
		return state, false, false, true
	case "enter":
		return state, n > 0, n > 0, true
	case "esc":
		return state, true, false, true
	case "backspace":
		if r := []rune(state.Filter); len(r) > 0 {
			state.Filter = string(r[:len(r)-1])
		}
		state.Cursor = clampCursor(state.Cursor, len(state.Visible()))
		return state, false, false, true
	}
	if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
		state.Filter += msg.Text
		state.Cursor = clampCursor(state.Cursor, len(state.Visible()))
		return state, false, false, true
	}
	return state, false, false, false
}

func menuNavBinding() key.Binding {
	return key.NewBinding(key.WithHelp("ctrl+n/ctrl+p or ↑/↓", "navigate"))
}

func menuFilterBinding() key.Binding {
	return key.NewBinding(key.WithHelp("type", "filter"))
}

func menuSelectBinding() key.Binding {
	return key.NewBinding(key.WithHelp("enter", "select"))
}

func menuCancelBinding() key.Binding {
	return key.NewBinding(key.WithHelp("esc", "cancel"))
}

func RenderMenuList(state MenuState, subtleColor, selectedColor color.Color) string {
	selected := lipgloss.NewStyle().Foreground(selectedColor).Bold(true)
	normal := lipgloss.NewStyle().Foreground(subtleColor)

	visible := state.Visible()
	menuLines := make([]string, 0, len(visible)+1)
	if state.Filter != "" {
		menuLines = append(menuLines, normal.Render("filter: ")+state.Filter)
	}
	if len(visible) == 0 {
		menuLines = append(menuLines, normal.Render("  no matches"))
	}
	for i, it := range visible {
		line := "  " + it.Label
		if i == state.Cursor {
			line = selected.Render("▸ " + it.Label)
		}
		if strings.TrimSpace(it.Detail) != "" {
			line += "  " + normal.Render(it.Detail)
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
