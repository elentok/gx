package log

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/nav"

	tea "charm.land/bubbletea/v2"
)

type reloadMsg struct {
	rows           []row
	branchDiverged bool
	err            error
}

func (m *Model) reload() {
	entries, err := git.LogEntries(m.worktreeRoot, m.startRef, maxLogEntries)
	if err != nil {
		m.err = err
		return
	}
	m.err = nil
	classes, branchDiverged := m.branchHistoryClasses()

	rows := make([]row, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, row{kind: rowCommit, commit: entry, class: classes[entry.FullHash]})
	}
	m.rows = rows
	m.branchDiverged = branchDiverged
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.recomputeSearchMatches()
}

func (m Model) cmdReload() tea.Cmd {
	worktreeRoot := m.worktreeRoot
	startRef := m.startRef
	return func() tea.Msg {
		entries, err := git.LogEntries(worktreeRoot, startRef, maxLogEntries)
		if err != nil {
			return reloadMsg{err: err}
		}
		classes, branchDiverged := fetchBranchHistoryClasses(worktreeRoot, startRef)
		rows := make([]row, 0, len(entries))
		for _, entry := range entries {
			rows = append(rows, row{kind: rowCommit, commit: entry, class: classes[entry.FullHash]})
		}
		return reloadMsg{rows: rows, branchDiverged: branchDiverged}
	}
}

func (m Model) branchHistoryClasses() (map[string]git.BranchHistoryClass, bool) {
	return fetchBranchHistoryClasses(m.worktreeRoot, m.startRef)
}

func fetchBranchHistoryClasses(worktreeRoot, startRef string) (map[string]git.BranchHistoryClass, bool) {
	if startRef != "HEAD" {
		return nil, false
	}

	branch, err := git.CurrentBranch(worktreeRoot)
	if err != nil {
		return nil, false
	}
	branch = normalizedRef(branch)
	if branch == "HEAD" {
		return nil, false
	}

	branchDiverged := false
	if upstream := git.UpstreamBranch(worktreeRoot, branch); upstream != "" {
		if sync, err := git.BranchSyncStatusAgainstRef(worktreeRoot, branch, upstream); err == nil {
			branchDiverged = sync.Name == git.StatusDiverged
		}
	}

	repo, err := git.FindRepo(worktreeRoot)
	if err != nil {
		return nil, branchDiverged
	}

	history, err := git.BranchHistorySinceMain(*repo, branch, git.UpstreamBranch(worktreeRoot, branch))
	if err != nil {
		return nil, branchDiverged
	}

	classes := make(map[string]git.BranchHistoryClass, len(history))
	for _, commit := range history {
		classes[commit.FullHash] = commit.Class
	}
	return classes, branchDiverged
}

func (m Model) openSelected() tea.Cmd {
	if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	selected := m.rows[m.cursor]
	return nav.Push(nav.Route{Kind: nav.RouteCommit, WorktreeRoot: m.worktreeRoot, Ref: selected.commit.FullHash})
}
