package notifyhistory

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/search"
)

// View composes the modal (centered) and, while /-search is mid-input, its
// search box (bottom-center) over bg, mirroring how ui/tickets' queue tab
// layers its own search overlay.
func (m Model) View(bg string, screenW, screenH int) string {
	frame := m.renderFrame(screenW, screenH)
	content := ui.OverlayCenter(bg, frame, screenW, screenH)

	if m.search.Mode() == search.SearchModeInput {
		overlayW := m.searchOverlayWidth(screenW)
		activeSearch := m.search
		activeSearch.SetWidth(overlayW)
		searchView := activeSearch.View()
		y := max(screenH-lipgloss.Height(searchView)-1, 0)
		content = ui.OverlayBottomCenter(content, searchView, screenW, y)
	}
	return content
}

func (m Model) searchOverlayWidth(screenW int) int {
	return min(search.DESIRED_WIDTH, screenW*80/100)
}

func (m Model) renderFrame(screenW, screenH int) string {
	width := modalWidth(screenW)
	bodyH := bodyHeight(screenH)
	visible := m.visibleEntries()
	scroll := clampScroll(m.scroll, len(visible), bodyH)

	innerW := max(width-4, 10)

	rows := make([]string, 0, bodyH)
	if len(visible) == 0 {
		rows = append(rows, "(no notifications)")
	}
	for i := scroll; i < len(visible) && len(rows) < bodyH; i++ {
		rows = append(rows, ansi.Truncate(entryText(visible[i]), innerW, "…"))
	}

	rightTitle := fmt.Sprintf("%d/%d", len(visible), len(m.entries))
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         "Notifications",
		RightTitle:    rightTitle,
		Body:          strings.Join(rows, "\n"),
		Hint:          "esc close · / search · ww export",
		Width:         innerW,
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		HintColor:     ui.ColorSubtle,
		TitleInBorder: true,
	})
}

func modalWidth(screenW int) int {
	w := min(screenW-8, 100)
	w = max(w, 20)
	return w
}

func bodyHeight(screenH int) int {
	return max(screenH-6, 3)
}
