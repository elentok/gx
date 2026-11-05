package worktrees

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	uisearch "github.com/elentok/gx/ui/search"
)

// searchModal is the incremental worktree search. It owns the query and match
// state; moving the table to a match is the page's job, so each state change
// that needs it comes back as an apply.
type searchModal struct {
	model uisearch.Model
}

// enterSearchMode opens the search modal with an empty query. Model.search is
// the closed template each open copies.
func (m Model) enterSearchMode() Model {
	s := m.search
	s.Start("")
	return m.openModal(searchModal{model: s})
}

// exitSearchMode closes the search modal and clears its highlights.
func (m Model) exitSearchMode() Model {
	m.modal = nil
	m.table.SetRows(m.buildRows())
	return m
}

// searchQuery is the open search's query, empty when no search is open.
func (m Model) searchQuery() string {
	if s, ok := m.modal.(searchModal); ok {
		return s.model.Query()
	}
	return ""
}

func (searchModal) bottomAnchored() {}

func (searchModal) InputFocused() bool { return true }

// View renders the search input overlay.
func (s searchModal) View(width, _ int) string {
	s.model.SetWidth(min(uisearch.DESIRED_WIDTH, width*80/100))
	return s.model.View()
}

// Update closes on esc/enter, steps through matches on ctrl+n/ctrl+p, and
// feeds every other key to the search input.
func (s searchModal) Update(msg tea.Msg) (modal, tea.Cmd, modalResult) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		s.model.Start(msg.Content)
		return s, nil, modalResult{apply: Model.updateSearchMatches}
	case tea.KeyPressMsg:
		return s.updateKey(msg)
	}
	return s, nil, modalResult{unhandled: true}
}

func (s searchModal) updateKey(msg tea.KeyPressMsg) (modal, tea.Cmd, modalResult) {
	switch msg.String() {
	case "esc", "enter":
		return s, nil, modalResult{close: true, apply: func(m Model) (Model, tea.Cmd) {
			return m.exitSearchMode(), nil
		}}
	case "ctrl+n":
		if n := s.model.MatchesCount(); n > 0 {
			return s.stepTo((s.model.Cursor() + 1) % n)
		}
		return s, nil, modalResult{}
	case "ctrl+p":
		if n := s.model.MatchesCount(); n > 0 {
			return s.stepTo((s.model.Cursor() - 1 + n) % n)
		}
		return s, nil, modalResult{}
	}

	next, cmd, result := s.model.Update(msg)
	s.model = next
	if !result.Handled {
		return s, nil, modalResult{}
	}
	if result.QueryChanged {
		return s, nil, modalResult{apply: Model.updateSearchMatches}
	}
	if result.CursorChanged {
		return s, nil, modalResult{apply: Model.jumpToCurrentMatch}
	}
	return s, cmd, modalResult{}
}

func (s searchModal) stepTo(idx int) (modal, tea.Cmd, modalResult) {
	s.model.SetCursor(idx)
	return s, nil, modalResult{apply: Model.jumpToCurrentMatch}
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

// updateSearchMatches recomputes the open search's matches and jumps to the
// current one.
func (m Model) updateSearchMatches() (Model, tea.Cmd) {
	s, ok := m.modal.(searchModal)
	if !ok {
		return m, nil
	}
	matches := m.computeSearchMatches(s.model.Query())
	s.model.SetMatches(matches)
	m.modal = s
	if len(matches) > 0 {
		if match, ok := s.model.Match(s.model.Cursor()); ok {
			return m.jumpToSearchMatch(match)
		}
	}
	m.table.SetRows(m.buildRows())
	return m, nil
}

// jumpToCurrentMatch moves the table to the open search's current match.
func (m Model) jumpToCurrentMatch() (Model, tea.Cmd) {
	if s, ok := m.modal.(searchModal); ok {
		if match, ok := s.model.Match(s.model.Cursor()); ok {
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
