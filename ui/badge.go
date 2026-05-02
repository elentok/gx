package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

type BadgeVariant string

const (
	BadgeVariantSurface BadgeVariant = "surface"
	BadgeVariantBlue    BadgeVariant = "blue"
	BadgeVariantGreen   BadgeVariant = "green"
	BadgeVariantYellow  BadgeVariant = "yellow"
	BadgeVariantOrange  BadgeVariant = "orange"
	BadgeVariantMauve   BadgeVariant = "mauve"
)

// RenderBadge returns a pill-shaped badge with a Catppuccin variant color theme.
func RenderBadge(label string, variant BadgeVariant, nerd bool) string {
	return renderPill(label, badgeBackground(variant), badgeForeground(variant), false, nerd)
}

func renderPill(label string, bgColor color.Color, fgColor color.Color, bold bool, nerd bool) string {
	bodyStyle := lipgloss.NewStyle().
		Background(bgColor).
		Foreground(fgColor)
	if bold {
		bodyStyle = bodyStyle.Bold(true)
	}
	body := bodyStyle.Render(" " + strings.TrimSpace(label) + " ")

	if !nerd {
		return body
	}

	capStyle := lipgloss.NewStyle().Foreground(bgColor)
	return capStyle.Render(capLeft) + body + capStyle.Render(capRight)
}

func badgeBackground(variant BadgeVariant) color.Color {
	switch variant {
	case BadgeVariantBlue:
		return ColorBlue
	case BadgeVariantGreen:
		return ColorGreen
	case BadgeVariantYellow:
		return ColorYellow
	case BadgeVariantOrange:
		return ColorOrange
	case BadgeVariantMauve:
		return ColorMauve
	default:
		return ColorSurface
	}
}

func badgeForeground(variant BadgeVariant) color.Color {
	switch variant {
	case BadgeVariantSurface:
		return ColorSubtle
	default:
		return ColorDeepBg
	}
}
