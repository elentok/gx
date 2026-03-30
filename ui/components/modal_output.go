package components

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

func RenderOutputModal(title, body, hint string, borderColor, titleColor, subtleColor color.Color, width int) string {
	titleStyle := lipgloss.NewStyle().Foreground(titleColor).Bold(true)
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)
	if width > 0 {
		borderStyle = borderStyle.Width(width)
	}

	inner := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(title),
		"",
		body,
		"",
		lipgloss.NewStyle().Foreground(subtleColor).Render(hint),
	)
	return borderStyle.Render(inner)
}
