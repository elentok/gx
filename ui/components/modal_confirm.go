package components

import (
	"image/color"

	"gx/ui"

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

func RenderConfirmModal(prompt string, yes bool, borderColor, yesColor, noColor, subtleColor color.Color, width int) string {
	yesLabel := " Yes "
	noLabel := " No "
	if yes {
		yesLabel = ui.StyleStrong.Foreground(yesColor).Render("[Yes]")
		noLabel = ui.StyleMuted.Foreground(subtleColor).Render(" No ")
	} else {
		yesLabel = ui.StyleMuted.Foreground(subtleColor).Render(" Yes ")
		noLabel = ui.StyleStrong.Foreground(noColor).Render("[No]")
	}

	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Body:        prompt + "\n\n" + yesLabel + "  " + noLabel,
		Hint:        "h/l or y/n, enter confirm, esc cancel",
		Width:       width,
		BorderColor: borderColor,
		HintColor:   subtleColor,
	})
}
