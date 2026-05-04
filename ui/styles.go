package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Base palette. This follows the Catppuccin-inspired colors already used by the
// status UI so the rest of the app can share one visual language.
var (
	ColorBase    = lipgloss.Color("#1e1e2e")
	ColorDeepBg  = lipgloss.Color("#11111a")
	ColorText    = lipgloss.Color("#cdd6f4")
	ColorSubtle  = lipgloss.Color("#a6adc8")
	ColorBlue    = lipgloss.Color("#89b4fa")
	ColorGreen   = lipgloss.Color("#a6e3a1")
	ColorYellow  = lipgloss.Color("#f9e2af")
	ColorRed     = lipgloss.Color("#f38ba8")
	ColorOrange  = lipgloss.Color("#fab387")
	ColorMauve   = lipgloss.Color("#cba6f7")
	ColorTeal    = lipgloss.Color("#94e2d5")
	ColorSurface = lipgloss.Color("#313244")
)

// Backwards-compatible aliases still used by some call sites.
var (
	ColorGray    = ColorSubtle
	ColorBorder  = ColorSubtle
	ColorCyan    = ColorTeal
	ColorMagenta = ColorMauve
)

// Semantic text and surface styles.
var (
	StyleTitle       = lipgloss.NewStyle().Foreground(ColorBlue).Bold(true)
	StyleHeading     = lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	StyleHelpHeading = lipgloss.NewStyle().Foreground(ColorOrange).Bold(true)

	StyleStrong   = lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	StyleBody     = lipgloss.NewStyle().Foreground(ColorText)
	StyleMuted    = lipgloss.NewStyle().Foreground(ColorSubtle)
	StyleHint     = lipgloss.NewStyle().Foreground(ColorSubtle)
	StyleCodeLike = lipgloss.NewStyle().Foreground(ColorTeal)
)

// Status styles.
var (
	StyleStatusSynced   = lipgloss.NewStyle().Foreground(ColorGreen)
	StyleStatusAhead    = lipgloss.NewStyle().Foreground(ColorMauve)
	StyleStatusBehind   = lipgloss.NewStyle().Foreground(ColorYellow)
	StyleStatusDiverged = lipgloss.NewStyle().Foreground(ColorRed)
	StyleStatusUnknown  = lipgloss.NewStyle().Foreground(ColorSubtle)
)

// Text styles.
var (
	StyleBold         = lipgloss.NewStyle().Bold(true)
	StyleDim          = StyleMuted
	StyleRowHighlight = lipgloss.NewStyle().Background(ColorSurface)
)

// RenderRowHighlight applies the shared row highlight background and re-applies
// it after nested ANSI resets so per-cell foreground colors stay visible across
// the full row.
func RenderRowHighlight(text string) string {
	bg := backgroundANSI(ColorSurface)
	text = strings.ReplaceAll(text, "\x1b[0m", "\x1b[0m"+bg)
	text = strings.ReplaceAll(text, "\x1b[m", "\x1b[m"+bg)
	return bg + text + "\x1b[0m"
}

func backgroundANSI(c color.Color) string {
	nrgba := color.NRGBAModel.Convert(c).(color.NRGBA)
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", nrgba.R, nrgba.G, nrgba.B)
}
