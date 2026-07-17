package ui

import (
	"image/color"
)

// SeamColor is the background painted into the gap the layout draws
// between adjacent frameless panels (RenderSeamRow/RenderSeamColumn). It's
// ColorBase itself - lighter than the panel fill below - so the seam reads
// as a visible but low-contrast separator instead of a third, unrelated tone.
var SeamColor = ColorBase

// PanelOptionsFor builds PanelOptions, sharing the
// layering rules across every screen that renders the frame-free design
// (status, log, commit). sidebar marks a panel that is currently in sidebar
// mode (see CONTEXT.md) - shown alongside a detail panel rather than
// standalone - so it can render a slightly darker body fill than a
// standalone panel.
func PanelOptionsFor(width, height int, title, rightTitle string, lines []string, active bool, titleColor color.Color, accent color.Color, sidebar bool) PanelOptions {
	// Body fill is uniform across panels (no per-panel darkening); the
	// header row alone carries the active/inactive distinction, per HITL
	// feedback favoring a distinct header over a shifted panel body.
	headerBg := ColorSurface
	if active {
		headerBg = ColorSurface1
	}
	bodyBg := ColorBase
	if sidebar {
		bodyBg = ColorMantle
	}
	return PanelOptions{
		Width: width, Height: height, Title: title, RightTitle: rightTitle, Lines: lines,
		TitleColor: titleColor, TitleBold: active,
		// No margin: panels render edge-to-edge; separation between
		// adjacent panels is drawn by the layout as a seam (see
		// SeamColor), not baked into each panel.
		Background:       bodyBg,
		HeaderBackground: headerBg,
		PaddingX:         1, PaddingY: 0,
	}
}
