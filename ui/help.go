package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
)

const (
	MIN_WIDTH  = 56
	MAX_WIDTH  = 104
	MIN_HEIGHT = 8
)

func HelpViewportModel(contentWidth int, contentHeight int) viewport.Model {
	vpW := min(max(contentWidth*2/3, MIN_WIDTH), MAX_WIDTH)
	vpH := max(contentHeight/2-4, 8)
	return viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
}

type KeySection struct {
	Title    string
	Bindings []key.Binding
}

func NewKeySection(title string, bindings ...key.Binding) KeySection {
	return KeySection{Title: title, Bindings: bindings}
}

func RenderHelpView(sections []KeySection) string {
	keyStyle := StyleTitle
	descStyle := StyleHint
	sep := descStyle.Render("  ")

	var parts []string
	for _, section := range sections {
		heading := StyleHelpHeading.Render(section.Title)
		parts = append(parts, heading)
		for _, b := range section.Bindings {
			h := b.Help()
			parts = append(parts, "  "+keyStyle.Render(h.Key)+sep+descStyle.Render(h.Desc))
		}
		parts = append(parts, "")
	}

	var result strings.Builder
	for i, p := range parts {
		if i > 0 {
			result.WriteString("\n")
		}
		result.WriteString(p)
	}
	return result.String()
}
