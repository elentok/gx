package components

import (
	"image/color"

	"github.com/elentok/gx/ui"
)

func RenderInputModal(title, prompt, input, hint string, borderColor, titleColor, subtleColor color.Color, width int) string {
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Title:       title,
		Body:        prompt + "\n\n" + input,
		Hint:        hint,
		Width:       width,
		BorderColor: borderColor,
		TitleColor:  titleColor,
		HintColor:   subtleColor,
	})
}
