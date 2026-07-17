package worktrees

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	uisearch "github.com/elentok/gx/ui/search"
)

// enterSearchMode transitions the model into search mode with an empty query.
func (m Model) enterSearchMode() Model {
	m.mode = modeSearch
	m.search.Start("")
	return m
}

// exitSearchMode clears search state and returns to normal mode.
func (m Model) exitSearchMode() Model {
	m.mode = modeNormal
	m.search.DismissAndClear()
	m.table.SetRows(m.buildRows())
	return m
}

func (m Model) computeSearchMatches(query string) []uisearch.Match {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	matches := make([]uisearch.Match, 0)
	for i, wt := range m.worktrees {
		if strings.Contains(strings.ToLower(wt.Name), q) || strings.Contains(strings.ToLower(wt.Branch), q) {
			matches = append(matches, uisearch.Match{DataIndex: i})
		}
	}
	return matches
}

func (m Model) updateSearchMatches() (Model, tea.Cmd) {
	matches := m.computeSearchMatches(m.search.Query())
	m.search.SetMatches(matches)
	if len(matches) > 0 {
		if match, ok := m.search.Match(m.search.Cursor()); ok {
			return m.jumpToSearchMatch(match)
		}
	}
	m.table.SetRows(m.buildRows())
	return m, nil
}

// handleSearchKey handles key events while in search mode.
func (m Model) handleSearchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m = m.exitSearchMode()
		return m, nil
	case "ctrl+n":
		if m.search.MatchesCount() > 0 {
			nextIdx := (m.search.Cursor() + 1) % m.search.MatchesCount()
			m.search.SetCursor(nextIdx)
			if match, ok := m.search.Match(nextIdx); ok {
				return m.jumpToSearchMatch(match)
			}
		}
		return m, nil
	case "ctrl+p":
		if m.search.MatchesCount() > 0 {
			prevIdx := (m.search.Cursor() - 1 + m.search.MatchesCount()) % m.search.MatchesCount()
			m.search.SetCursor(prevIdx)
			if match, ok := m.search.Match(prevIdx); ok {
				return m.jumpToSearchMatch(match)
			}
		}
		return m, nil
	}

	next, cmd, result := m.search.Update(msg)
	m.search = next
	if !result.Handled {
		return m, nil
	}
	if result.QueryChanged {
		return m.updateSearchMatches()
	}
	if result.CursorChanged {
		if match, ok := m.search.Match(m.search.Cursor()); ok {
			return m.jumpToSearchMatch(match)
		}
	}
	return m, cmd
}

// jumpToSearchMatch moves the table cursor to the given search match and
// returns the preview-reload command.
func (m Model) jumpToSearchMatch(match uisearch.Match) (Model, tea.Cmd) {
	idx := match.DataIndex
	if idx < 0 || idx >= len(m.worktrees) {
		return m, nil
	}
	m.table.SetCursor(idx)
	m.table.SetRows(m.buildRows())
	return m, cmdLoadPreviewData(m.repo, m.worktrees[idx])
}
