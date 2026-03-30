package components

import (
	"image/color"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)
	if width > 0 {
		borderStyle = borderStyle.Width(width)
	}

	yesLabel := " Yes "
	noLabel := " No "
	if yes {
		yesLabel = lipgloss.NewStyle().Foreground(yesColor).Bold(true).Render("[Yes]")
		noLabel = lipgloss.NewStyle().Foreground(subtleColor).Render(" No ")
	} else {
		yesLabel = lipgloss.NewStyle().Foreground(subtleColor).Render(" Yes ")
		noLabel = lipgloss.NewStyle().Foreground(noColor).Bold(true).Render("[No]")
	}

	hint := lipgloss.NewStyle().Foreground(subtleColor).Render("h/l or y/n, enter confirm, esc cancel")
	inner := lipgloss.JoinVertical(lipgloss.Left,
		prompt,
		"",
		yesLabel+"  "+noLabel,
		hint,
	)
	return borderStyle.Render(inner)
}
