package ui

import (
	"fmt"
	"image/color"
	"strings"
)

// ResolveColor parses a color string into a color.Color.
// Accepts named Catppuccin colors (blue, green, yellow, orange, mauve, teal, red, surface)
// or hex strings (#rrggbb or #rgb).
func ResolveColor(s string) (color.Color, error) {
	switch strings.ToLower(s) {
	case "blue":
		return ColorBlue, nil
	case "green":
		return ColorGreen, nil
	case "yellow":
		return ColorYellow, nil
	case "orange":
		return ColorOrange, nil
	case "mauve":
		return ColorMauve, nil
	case "teal":
		return ColorTeal, nil
	case "red":
		return ColorRed, nil
	case "surface":
		return ColorSurface, nil
	}

	if strings.HasPrefix(s, "#") {
		return parseHexColor(s)
	}

	return nil, fmt.Errorf("unknown color %q", s)
}

func parseHexColor(s string) (color.Color, error) {
	s = strings.TrimPrefix(s, "#")
	switch len(s) {
	case 6:
		var r, g, b uint8
		_, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
		if err != nil {
			return nil, fmt.Errorf("invalid hex color #%s: %w", s, err)
		}
		return color.NRGBA{R: r, G: g, B: b, A: 255}, nil
	case 3:
		var r, g, b uint8
		_, err := fmt.Sscanf(s, "%01x%01x%01x", &r, &g, &b)
		if err != nil {
			return nil, fmt.Errorf("invalid hex color #%s: %w", s, err)
		}
		return color.NRGBA{R: r * 17, G: g * 17, B: b * 17, A: 255}, nil
	default:
		return nil, fmt.Errorf("invalid hex color #%s: must be 3 or 6 hex digits", s)
	}
}
