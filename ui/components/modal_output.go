package components

import (
	"image/color"

	"gx/ui"
)

func RenderOutputModal(title, body, hint string, borderColor, titleColor, subtleColor color.Color, width int) string {
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:       title,
		Body:        body,
		Hint:        hint,
		Width:       width,
		BorderColor: borderColor,
		TitleColor:  titleColor,
		HintColor:   subtleColor,
	})
}
