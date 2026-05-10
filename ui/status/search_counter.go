package status

import (
	"fmt"

	"github.com/elentok/gx/ui"
)

func (m Model) searchCounterForFiletreePane() string {
	search := m.fileTreeModel.Search()
	if m.focus != focusFiletree || !search.HasQuery() || search.MatchesCount() == 0 {
		return ""
	}
	return m.searchCounterText(search.Cursor(), search.MatchesCount())
}

func (m Model) searchCounterForDiffSection(section diffSection) string {
	search := m.diffSearchForSection(section)
	if !search.HasQuery() || search.MatchesCount() == 0 {
		return ""
	}

	if m.focus != focusDiff || m.section != section {
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
