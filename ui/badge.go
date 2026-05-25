package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

type BadgeVariant string

const (
	BadgeVariantSurface BadgeVariant = "surface"
	BadgeVariantDeepBg  BadgeVariant = "deepbg"
	BadgeVariantBlue    BadgeVariant = "blue"
	BadgeVariantGreen   BadgeVariant = "green"
	BadgeVariantYellow  BadgeVariant = "yellow"
	BadgeVariantOrange  BadgeVariant = "orange"
	BadgeVariantMauve   BadgeVariant = "mauve"
)

// RenderBadge returns a pill-shaped badge with a Catppuccin variant color theme.
// padding adds a space on each side of the label.
func RenderBadge(label string, variant BadgeVariant, nerd bool, padding bool) string {
	return renderPill(label, badgeBackground(variant), badgeForeground(variant), false, nerd, padding)
}

// RenderBadgeWithColor renders a pill badge using an explicit foreground color.
// padding adds a space on each side of the label.
func RenderBadgeWithColor(label string, fg color.Color, nerd bool, padding bool) string {
	return renderPill(label, ColorDeepBg, fg, false, nerd, padding)
}

func renderPill(label string, bgColor color.Color, fgColor color.Color, bold bool, nerd bool, padding bool) string {
	bodyStyle := lipgloss.NewStyle().
		Background(bgColor).
		Foreground(fgColor)
	if bold {
		bodyStyle = bodyStyle.Bold(true)
	}
	text := strings.TrimSpace(label)
	if padding {
		text = " " + text + " "
	}
	body := bodyStyle.Render(text)

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
	case BadgeVariantDeepBg:
		return ColorDeepBg
	default:
		return ColorSurface
	}
}

func badgeForeground(variant BadgeVariant) color.Color {
	switch variant {
	case BadgeVariantSurface:
		return ColorSubtle
	case BadgeVariantDeepBg:
		return ColorSubtle
	default:
		return ColorDeepBg
	}
}
