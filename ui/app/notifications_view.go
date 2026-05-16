package app

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

const notifyMaxContentWidth = 36 // max 40 cols total; 4 for border+padding

var (
	notifyBorderInfo     = ui.ColorBlue
	notifyBorderSuccess  = ui.ColorGreen
	notifyBorderWarning  = ui.ColorOrange
	notifyBorderError    = ui.ColorRed
	notifyBorderProgress = ui.ColorTeal
)

func (m *Model) renderNotificationStack(useNerdFont bool) string {
	if len(m.notifications) == 0 {
		return ""
	}
	icons := ui.Icons(useNerdFont)
	parts := make([]string, 0, len(m.notifications))
	for _, n := range m.notifications {
		parts = append(parts, renderNotification(n, icons, m.spinner.View()))
	}
	return strings.Join(parts, "\n")
}

func renderNotification(n notification, icons ui.IconSet, spinnerFrame string) string {
	icon, borderColor := notifyIconAndColor(n, icons, spinnerFrame)
	body := icon + " " + n.message

	// Wrap long messages.
	if lipgloss.Width(body) > notifyMaxContentWidth {
		body = icon + " " + wrapText(n.message, notifyMaxContentWidth-lipgloss.Width(icon)-1)
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Foreground(ui.ColorText).
		Padding(0, 1)

	return style.Render(body)
}

func notifyIconAndColor(n notification, icons ui.IconSet, spinnerFrame string) (string, color.Color) {
	switch n.kind {
	case notify.KindSuccess:
		return lipgloss.NewStyle().Foreground(notifyBorderSuccess).Render(icons.Check), notifyBorderSuccess
	case notify.KindWarning:
		return lipgloss.NewStyle().Foreground(notifyBorderWarning).Render(icons.Warning), notifyBorderWarning
	case notify.KindError:
		return lipgloss.NewStyle().Foreground(notifyBorderError).Render(icons.Close), notifyBorderError
	case notify.KindProgress:
		return lipgloss.NewStyle().Foreground(notifyBorderProgress).Render(spinnerFrame), notifyBorderProgress
	default: // KindInfo
		return lipgloss.NewStyle().Foreground(notifyBorderInfo).Render("i"), notifyBorderInfo
	}
}

// wrapText wraps s at maxWidth columns, breaking on spaces.
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
