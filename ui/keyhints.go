package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
)

// RenderInlineBindings renders compact "key description" hints for transient
// status lines and prompts.
func RenderInlineBindings(bindings ...key.Binding) string {
	parts := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		help := binding.Help()
		keyLabel := help.Key
		desc := help.Desc
		switch {
		case keyLabel == "" && desc == "":
			continue
		case desc == "":
			parts = append(parts, StyleTitle.Render(keyLabel))
		case keyLabel == "":
			parts = append(parts, StyleHint.Render(desc))
		default:
			parts = append(parts, StyleTitle.Render(keyLabel)+" "+StyleHint.Render(desc))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, StyleHint.Render(" · "))
}
