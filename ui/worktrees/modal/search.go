package modal

import (
	"strings"

	"github.com/elentok/gx/git"

	tea "charm.land/bubbletea/v2"
	uisearch "github.com/elentok/gx/ui/search"
)

// Search is the incremental worktree search. It owns the query and match
// state; moving the table to a match is the page's job, so each state change
// that needs it comes back as an action.
type Search struct {
	model uisearch.Model
}

// NewSearch opens a search with an empty query. template is the closed search
// model each open copies.
func NewSearch(template uisearch.Model) Search {
	template.Start("")
	return Search{model: template}
}

// Query is the current query.
func (s Search) Query() string { return s.model.Query() }

// Matches is the current matches.
func (s Search) Matches() []uisearch.Match { return s.model.Matches() }

// Cursor is the index of the current match.
func (s Search) Cursor() int { return s.model.Cursor() }

// Match is the match at index i.
func (s Search) Match(i int) (uisearch.Match, bool) { return s.model.Match(i) }

// CurrentMatch is the match under the cursor.
func (s Search) CurrentMatch() (uisearch.Match, bool) { return s.model.Match(s.model.Cursor()) }

// Recompute matches the query against the worktrees' names and branches.
func (s Search) Recompute(worktrees []git.Worktree) Search {
	s.model.SetMatches(computeSearchMatches(worktrees, s.model.Query()))
	return s
}

func (Search) BottomAnchored() {}

func (Search) InputFocused() bool { return true }

// View renders the search input overlay.
func (s Search) View(width, _ int) string {
	s.model.SetWidth(min(uisearch.DESIRED_WIDTH, width*80/100))
	return s.model.View()
}

// Update closes on esc/enter, steps through matches on ctrl+n/ctrl+p, and
// feeds every other key to the search input.
func (s Search) Update(msg tea.Msg) (Modal, tea.Cmd, Result) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		s.model.Start(msg.Content)
		return s, nil, Result{Action: SearchQueryChanged{}}
	case tea.KeyPressMsg:
		return s.updateKey(msg)
	}
	return s, nil, Result{Unhandled: true}
}

func (s Search) updateKey(msg tea.KeyPressMsg) (Modal, tea.Cmd, Result) {
	switch msg.String() {
	case "esc", "enter":
		return s, nil, Result{Close: true, Action: SearchClosed{}}
	case "ctrl+n":
		if n := s.model.MatchesCount(); n > 0 {
			return s.stepTo((s.model.Cursor() + 1) % n)
		}
		return s, nil, Result{}
	case "ctrl+p":
		if n := s.model.MatchesCount(); n > 0 {
			return s.stepTo((s.model.Cursor() - 1 + n) % n)
		}
		return s, nil, Result{}
	}

	next, cmd, result := s.model.Update(msg)
	s.model = next
	if !result.Handled {
		return s, nil, Result{}
	}
	if result.QueryChanged {
		return s, nil, Result{Action: SearchQueryChanged{}}
	}
	if result.CursorChanged {
		return s, nil, Result{Action: SearchCursorMoved{}}
	}
	return s, cmd, Result{}
}

func (s Search) stepTo(idx int) (Modal, tea.Cmd, Result) {
	s.model.SetCursor(idx)
	return s, nil, Result{Action: SearchCursorMoved{}}
}

func computeSearchMatches(worktrees []git.Worktree, query string) []uisearch.Match {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	matches := make([]uisearch.Match, 0)
	for i, wt := range worktrees {
		if strings.Contains(strings.ToLower(wt.Name), q) || strings.Contains(strings.ToLower(wt.Branch), q) {
			matches = append(matches, uisearch.Match{DataIndex: i})
		}
	}
	return matches
}
