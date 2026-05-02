package log

import (
	"fmt"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) reload() {
	entries, err := git.LogEntries(m.worktreeRoot, m.startRef, maxLogEntries)
	if err != nil {
		m.err = err
		return
	}
	m.err = nil

	rows := make([]row, 0, len(entries)+1)
	if m.startRef == "HEAD" {
		changes, changeErr := git.UncommittedChanges(m.worktreeRoot)
		if changeErr == nil && len(changes) > 0 {
			rows = append(rows, row{
				kind:   rowPseudoStatus,
				label:  "working tree",
				detail: fmt.Sprintf("%d uncommitted change(s)", len(changes)),
			})
		}
	}
	for _, entry := range entries {
		rows = append(rows, row{kind: rowCommit, commit: entry})
	}
	m.rows = rows
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.recomputeSearchMatches()
}

func (m Model) openSelected() tea.Cmd {
	if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	selected := m.rows[m.cursor]
	if selected.kind == rowPseudoStatus {
		return nav.Push(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: m.worktreeRoot})
	}
	return nav.Push(nav.Route{Kind: nav.RouteCommit, WorktreeRoot: m.worktreeRoot, Ref: selected.commit.FullHash})
}
