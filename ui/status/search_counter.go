package status

import (
	"fmt"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/status/diffarea"
)

func (m Model) searchCounterForFiletreePane() string {
	search := m.fileTreeModel.Search()
	if m.focus != focusFiletree || !search.HasQuery() || search.MatchesCount() == 0 {
		return ""
	}
	return m.searchCounterText(search.Cursor(), search.MatchesCount())
}

func (m Model) searchCounterForDiffSection(section diffarea.Section) string {
	search := m.diffSearchForSection(section)
	if !search.HasQuery() || search.MatchesCount() == 0 {
		return ""
	}

	if m.focus != focusDiff || m.diff.ActiveSection != section {
		return ""
	}
	return m.searchCounterText(search.Cursor(), search.MatchesCount())
}

func (m Model) searchCounterText(cursorZeroBased, total int) string {
	if total == 0 {
		return ""
	}
	cursor := cursorZeroBased + 1
	if cursor < 1 {
		cursor = 1
	}
	if cursor > total {
		cursor = total
	}

	icon := "⌕"
	if m.settings.UseNerdFontIcons {
		icon = ui.Icons(true).Search
	}
	return fmt.Sprintf("%s %d/%d", icon, cursor, total)
}
