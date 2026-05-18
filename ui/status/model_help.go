package status

import (
	"fmt"

	"github.com/elentok/gx/ui/diffview"
)

func (m Model) helpSectionLabel() string {
	if m.focus == focusFiletree {
		return "filetree"
	}
	return fmt.Sprintf("diff:%s:%s", m.navModeLabel(), m.renderModeLabel())
}

func (m Model) navModeLabel() string {
	if m.diffarea.NavMode() == diffview.NavModeLine {
		return "line"
	}
	return "hunk"
}
