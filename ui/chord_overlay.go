package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ChordHinter is implemented by models that expose chord completion hints.
// The app model queries the active child via this interface to combine app-level
// and child-level hints into one overlay.
type ChordHinter interface {
	ChordHints(prefix string) []key.Binding
}

// RenderChordOverlay renders a compact box listing the available chord completions
// for the given prefix key. The prefix is embedded in the top border and the
// "esc close" hint is right-aligned. Intended for placement in the top-right
// corner via OverlayTopRight.
func RenderChordOverlay(prefix string, bindings []key.Binding) string {
	arrow := "➜"

	type row struct {
		keyLabel string
		desc     string
	}
	rows := make([]row, 0, len(bindings))
	maxKeyW := 0
	for _, b := range bindings {
		h := b.Help()
		if h.Key == "" && h.Desc == "" {
			continue
		}
		rows = append(rows, row{keyLabel: h.Key, desc: h.Desc})
		if w := ansi.StringWidth(h.Key); w > maxKeyW {
			maxKeyW = w
		}
	}

	if len(rows) == 0 {
		return ""
	}

	closeHint := StyleMuted.Render("esc") + " " + StyleHint.Render("close")
	closeHintW := ansi.StringWidth(closeHint)

	innerW := closeHintW
	contentLines := make([]string, 0, len(rows)+1)
	for _, r := range rows {
		pad := strings.Repeat(" ", maxKeyW-ansi.StringWidth(r.keyLabel))
		line := StyleTitle.Render(r.keyLabel) + pad + " " + StyleMuted.Render(arrow) + " " + StyleHint.Render(r.desc)
		contentLines = append(contentLines, line)
		if w := ansi.StringWidth(line); w > innerW {
			innerW = w
		}
	}

	leftPad := strings.Repeat(" ", innerW-closeHintW)
	contentLines = append(contentLines, leftPad+closeHint)

	body := strings.Join(contentLines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1).
		Render(body)
	return injectBorderTitle(box, prefix, "", ColorBlue, ColorBorder, ColorBorder)
}
