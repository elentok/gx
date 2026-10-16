package status

import (
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/keys"
)

// diffHeaderHelpSection explains the diff panel header's indicators, which
// aren't key bindings so BuildSections can't derive them.
func diffHeaderHelpSection() help.KeySection {
	return help.NewKeySection("Diff header", keys.Binding{Display: "±N", Title: "context lines around each change"})
}

func (m Model) navModeLabel() string {
	if m.diffarea.NavMode() == diffview.NavModeLine {
		return "line"
	}
	return "hunk"
}
