package status

import (
	"image/color"

	"github.com/elentok/gx/ui"
)

func (m Model) renderFiletreePanelWithBorderTitle(width, height int, title, rightTitle string, lines []string, active bool) string {
	titleColor := ui.ColorBlue
	accent := color.Color(nil)
	if active {
		accent = ui.ColorBlue
	}
	return ui.RenderPanel(ui.PanelOptionsFor(width, height, title, rightTitle, lines, active, titleColor, accent, true))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
