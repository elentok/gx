package worktrees

import (
	"github.com/elentok/gx/ui/worktrees/modal"

	tea "charm.land/bubbletea/v2"
	uisearch "github.com/elentok/gx/ui/search"
)

// enterSearchMode opens the search modal with an empty query. Model.search is
// the closed template each open copies.
func (m Model) enterSearchMode() Model {
	return m.openModal(modal.NewSearch(m.search))
}

// exitSearchMode closes the search modal and clears its highlights.
func (m Model) exitSearchMode() Model {
	m.modal = nil
	m.table.SetRows(m.buildRows())
	return m
}

// searchQuery is the open search's query, empty when no search is open.
func (m Model) searchQuery() string {
	if s, ok := m.modal.(modal.Search); ok {
		return s.Query()
	}
	return ""
}

// updateSearchMatches recomputes the open search's matches and jumps to the
// current one.
func (m Model) updateSearchMatches() (Model, tea.Cmd) {
	s, ok := m.modal.(modal.Search)
	if !ok {
		return m, nil
	}
	s = s.Recompute(m.worktrees)
	m.modal = s
	if match, ok := s.CurrentMatch(); ok {
		return m.jumpToSearchMatch(match)
	}
	m.table.SetRows(m.buildRows())
	return m, nil
}

// jumpToCurrentMatch moves the table to the open search's current match.
func (m Model) jumpToCurrentMatch() (Model, tea.Cmd) {
	if s, ok := m.modal.(modal.Search); ok {
		if match, ok := s.CurrentMatch(); ok {
			return m.jumpToSearchMatch(match)
		}
	}
	return m, nil
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
