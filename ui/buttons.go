package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

const (
	// Powerline "soft" rounded separators for pill-shaped buttons.
	capLeft  = "\uE0B6"
	capRight = "\uE0B4"
)

// RenderButton returns a styled button string.
// If selected, the button is highlighted (green, bold).
// If nerd is true, powerline rounded caps are added so the button looks like a pill.
func RenderButton(label string, selected bool, nerd bool) string {
	var bgColor color.Color
	var fgColor color.Color
	if selected {
		bgColor = ColorGreen
		fgColor = lipgloss.Color("0")
	} else {
		bgColor = lipgloss.Color("240")
		fgColor = lipgloss.Color("7")
	}

	return renderPill(label, bgColor, fgColor, selected, nerd)
}
