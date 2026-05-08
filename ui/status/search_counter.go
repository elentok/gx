package status

import (
	"fmt"

	"github.com/elentok/gx/ui"
)

func (m Model) searchCounterForStatusPane() string {
	if m.currentSearchScope() != searchScopeStatus || !m.search.HasQuery() || m.search.MatchesCount() == 0 {
		return ""
	}
	return m.searchCounterText()
}

func (m Model) searchCounterForDiffSection(section diffSection) string {
	if !m.search.HasQuery() || m.search.MatchesCount() == 0 {
		return ""
	}

	expected := searchScopeUnstaged
	if section == sectionStaged {
		expected = searchScopeStaged
	}
	if m.currentSearchScope() != expected {
		return ""
	}
	return m.searchCounterText()
}

func (m Model) searchCounterText() string {
	total := m.search.MatchesCount()
	if total == 0 {
		return ""
	}
	cursor := m.search.Cursor() + 1
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
