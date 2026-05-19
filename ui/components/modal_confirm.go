package components

import (
	"image/color"
	"strings"

	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// UpdateConfirm applies standard yes/no confirm key handling.
// Returns nextYes, decided, accepted, handled.
func UpdateConfirm(msg tea.KeyPressMsg, yes bool) (bool, bool, bool, bool) {
	switch msg.String() {
	case "left", "h":
		return true, false, false, true
	case "right", "l":
		return false, false, false, true
	case "y":
		return true, true, true, true
	case "n", "esc", "q":
		return false, true, false, true
	case "enter":
		return yes, true, yes, true
	default:
		return yes, false, false, false
	}
}

var ConfirmHint = ui.RenderInlineBindings(
	key.NewBinding(key.WithHelp("h/l or ←/→", "choose")),
	key.NewBinding(key.WithHelp("y/n", "quick select")),
	key.NewBinding(key.WithHelp("enter", "confirm")),
	key.NewBinding(key.WithHelp("esc", "cancel")),
)

func RenderConfirmChoices(yes bool, nerd bool) string {
	return "  " + ui.RenderButton("Yes", yes, nerd) + "   " + ui.RenderButton("No", !yes, nerd)
}

func RenderConfirmContent(prompt string, yes bool, nerd bool, items ...string) string {
	var sb strings.Builder
	sb.WriteString(prompt)
	if len(items) > 0 {
		sb.WriteString("\n")
		for _, item := range items {
			sb.WriteString("\n  • ")
			sb.WriteString(item)
		}
	}
	sb.WriteString("\n\n")
	sb.WriteString(RenderConfirmChoices(yes, nerd))
	return sb.String()
}

func RenderConfirmModal(prompt string, yes bool, borderColor, yesColor, noColor, subtleColor color.Color, width int, items ...string) string {
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Body:        RenderConfirmContent(prompt, yes, false, items...),
		Hint:        ConfirmHint,
		Width:       width,
		BorderColor: borderColor,
		HintColor:   subtleColor,
	})
}
