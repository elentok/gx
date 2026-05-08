package search

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/ui"
)

func (m *Model) View() string {
	var rightTitle string
	total := len(m.matches)
	if strings.TrimSpace(m.query) != "" {
		if total == 0 {
			rightTitle = "no matches"
		} else {
			rightTitle = fmt.Sprintf("%d/%d", m.cursor+1, total)
		}
	}

	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:         "Search",
		RightTitle:    rightTitle,
		Body:          m.textinput.View(),
		Width:         m.width,
		BorderColor:   ui.ColorBorder,
		TitleColor:    ui.ColorBlue,
		TitleInBorder: true,
	})
}

const (
	DESIRED_WIDTH = 50
)
