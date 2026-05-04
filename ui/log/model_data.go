package log

import (
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
	classes := m.branchHistoryClasses()

	rows := make([]row, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, row{kind: rowCommit, commit: entry, class: classes[entry.FullHash]})
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

func (m Model) branchHistoryClasses() map[string]git.BranchHistoryClass {
	if m.startRef != "HEAD" {
		return nil
	}

	branch, err := git.CurrentBranch(m.worktreeRoot)
	if err != nil {
		return nil
	}
	branch = normalizedRef(branch)
	if branch == "HEAD" {
		return nil
	}

	repo, err := git.FindRepo(m.worktreeRoot)
	if err != nil {
		return nil
	}

	history, err := git.BranchHistorySinceMain(*repo, branch, git.UpstreamBranch(m.worktreeRoot, branch))
	if err != nil {
		return nil
	}

	classes := make(map[string]git.BranchHistoryClass, len(history))
	for _, commit := range history {
		classes[commit.FullHash] = commit.Class
	}
	return classes
}

func (m Model) openSelected() tea.Cmd {
	if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	selected := m.rows[m.cursor]
	return nav.Push(nav.Route{Kind: nav.RouteCommit, WorktreeRoot: m.worktreeRoot, Ref: selected.commit.FullHash})
}
