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

// RenderBadgeText renders a label as plain colored text with no background
// pill. Use it where badges sit inline among other plain text (e.g. next to
// a commit subject): a background box there would make the subject start at
// a different column depending on whether a row has decorations, which reads
// as misaligned.
func RenderBadgeText(label string, fg color.Color) string {
	return lipgloss.NewStyle().Foreground(fg).Render(strings.TrimSpace(label))
}

// BadgeGroupItem is one decoration name plus its foreground color, to be
// rendered as part of a merged badge group.
type BadgeGroupItem struct {
	Label string
	Fg    color.Color
}

// RenderBadgeGroup renders multiple decorations as a single merged pill: one
// shared neutral background spans all names joined by spaces, with each name
// keeping its own foreground color. Used by condensed log rows in place of
// one separate badge per decoration.
func RenderBadgeGroup(items []BadgeGroupItem, nerd bool) string {
	if len(items) == 0 {
		return ""
	}
	bg := ColorDeepBg
	sep := lipgloss.NewStyle().Background(bg).Render(" ")
	parts := make([]string, 0, len(items))
	for _, item := range items {
		style := lipgloss.NewStyle().Background(bg).Foreground(item.Fg)
		parts = append(parts, style.Render(strings.TrimSpace(item.Label)))
	}
	body := strings.Join(parts, sep)

	if !nerd {
		return body
	}
	capStyle := lipgloss.NewStyle().Foreground(bg)
	return capStyle.Render(capLeft) + body + capStyle.Render(capRight)
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
