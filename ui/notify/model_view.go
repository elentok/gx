package notify

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/elentok/gx/ui"
)

const maxContentWidth = 36 // max 40 cols total; 4 for border+padding

var (
	colorInfo     = ui.ColorBlue
	colorSuccess  = ui.ColorGreen
	colorWarning  = ui.ColorOrange
	colorError    = ui.ColorRed
	colorProgress = ui.ColorTeal
)

func (m Model) View() string {
	if len(m.notifications) == 0 {
		return ""
	}
	icons := ui.Icons(m.useNerdFont)
	parts := make([]string, 0, len(m.notifications))
	for _, n := range m.notifications {
		parts = append(parts, renderNotification(n, icons, m.spinner.View()))
	}
	return strings.Join(parts, "\n")
}

func renderNotification(n notification, icons ui.IconSet, spinnerFrame string) string {
	icon, borderColor := iconAndColor(n, icons, spinnerFrame)
	body := icon + " " + n.message

	if lipgloss.Width(body) > maxContentWidth {
		body = icon + " " + wrapText(n.message, maxContentWidth-lipgloss.Width(icon)-1)
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Foreground(ui.ColorText).
		Padding(0, 1)

	return style.Render(body)
}

func iconAndColor(n notification, icons ui.IconSet, spinnerFrame string) (string, color.Color) {
	switch n.kind {
	case KindSuccess:
		return lipgloss.NewStyle().Foreground(colorSuccess).Render(icons.Check), colorSuccess
	case KindWarning:
		return lipgloss.NewStyle().Foreground(colorWarning).Render(icons.Warning), colorWarning
	case KindError:
		return lipgloss.NewStyle().Foreground(colorError).Render(icons.Close), colorError
	case KindProgress:
		return lipgloss.NewStyle().Foreground(colorProgress).Render(spinnerFrame), colorProgress
	default: // KindInfo
		return lipgloss.NewStyle().Foreground(colorInfo).Render(icons.Info), colorInfo
	}
}

func wrapText(s string, maxWidth int) string {
	if maxWidth <= 0 || lipgloss.Width(s) <= maxWidth {
		return s
	}
	words := strings.Fields(s)
	var lines []string
	current := ""
	for _, w := range words {
		candidate := w
		if current != "" {
			candidate = current + " " + w
		}
		if lipgloss.Width(candidate) > maxWidth && current != "" {
			lines = append(lines, current)
			current = w
		} else {
			current = candidate
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}
