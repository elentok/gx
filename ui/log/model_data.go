package log

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

type reloadMsg struct {
	rows           []row
	branchDiverged bool
	err            error
	focusSubject   string // if set, cursor is moved to first matching commit
}

type worktreeStatusMsg struct {
	staged    int
	unstaged  int
	untracked int
	err       error
}

func (m Model) cmdLoadStatus() tea.Cmd {
	root := m.worktreeRoot
	return func() tea.Msg {
		staged, unstaged, untracked, err := git.WorktreeStatusSummary(root)
		return worktreeStatusMsg{staged: staged, unstaged: unstaged, untracked: untracked, err: err}
	}
}

// pseudoStatusDetail formats the worktree status text for the pseudo-log-line.
func (m Model) pseudoStatusDetail() string {
	if !m.statusLoaded {
		return "loading worktree status…"
	}
	if m.statusStaged == 0 && m.statusUnstaged == 0 && m.statusUntracked == 0 {
		return "no local changes"
	}
	var parts []string
	if m.statusStaged > 0 {
		parts = append(parts, fmt.Sprintf("%d staged", m.statusStaged))
	}
	if m.statusUnstaged > 0 {
		parts = append(parts, fmt.Sprintf("%d unstaged", m.statusUnstaged))
	}
	if m.statusUntracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", m.statusUntracked))
	}
	return strings.Join(parts, " · ")
}

func (m Model) gitFilter() git.LogFilter {
	return git.LogFilter{
		Path:      m.filter.Path,
		StartLine: m.filter.StartLine,
		EndLine:   m.filter.EndLine,
	}
}

func (m Model) cmdReload() tea.Cmd {
	worktreeRoot := m.worktreeRoot
	startRef := m.startRef
	filter := m.gitFilter()
	statusDetail := m.pseudoStatusDetail()
	return func() tea.Msg {
		entries, err := git.LogEntriesFiltered(worktreeRoot, startRef, maxLogEntries, filter)
		if err != nil {
			return reloadMsg{err: err}
		}
		classes, branchDiverged := fetchBranchHistoryClasses(worktreeRoot, startRef)
		rows := make([]row, 0, len(entries)+1)
		rows = append(rows, row{kind: rowPseudoStatus, detail: statusDetail})
		for _, entry := range entries {
			rows = append(rows, row{kind: rowCommit, commit: entry, class: classes[entry.FullHash]})
		}
		return reloadMsg{rows: rows, branchDiverged: branchDiverged}
	}
}

func fetchBranchHistoryClasses(worktreeRoot, startRef string) (map[string]git.BranchHistoryClass, bool) {
	var branch string
	if startRef == "HEAD" {
		b, err := git.CurrentBranch(worktreeRoot)
		if err != nil {
			return nil, false
		}
		branch = normalizedRef(b)
		if branch == "HEAD" {
			return nil, false
		}
	} else if git.IsLocalBranch(worktreeRoot, startRef) {
		branch = startRef
	} else {
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

type gotoPRMsg struct {
	url string
	err error
}

func (m Model) cmdGotoPR() tea.Cmd {
	rows := m.listPanel.Rows()
	cursor := m.listPanel.Selected()
	if len(rows) == 0 || cursor < 0 || cursor >= len(rows) {
		return nil
	}
	selected := rows[cursor]
	worktreeRoot := m.worktreeRoot
	return func() tea.Msg {
		var url string
		var err error
		if selected.class == "" {
			url, err = git.CommitPRURL(worktreeRoot, selected.commit.FullHash)
		} else {
			url, err = git.BranchPRURL(worktreeRoot)
		}
		return gotoPRMsg{url: url, err: err}
	}
}

func (m Model) handleGotoPR(msg gotoPRMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil || msg.url == "" {
		return m, notify.Warning("no PR found")
	}
	return m, ui.CmdOpenURL(msg.url)
}

// openSelected handles Enter on the currently selected row.
// Returns (updatedModel, cmd) — the model is returned because entering split
// mode mutates the split container and commitDetail inline.
func (m Model) openSelected() (Model, tea.Cmd) {
	rows := m.listPanel.Rows()
	cursor := m.listPanel.Selected()
	if len(rows) == 0 || cursor < 0 || cursor >= len(rows) {
		return m, nil
	}
	selected := rows[cursor]

	if selected.kind == rowPseudoStatus {
		return m, nav.Switch(nav.ViewState{Tab: nav.TabStatus, WorktreeRoot: m.worktreeRoot})
	}

	// Real commit row: expand to split view.
	ref := selected.commit.FullHash
	if ref == "" {
		return m, nil
	}
	// Update the split list adapter ref so the container tracks the selection.
	m.split = m.split.WithListRef(ref)
	// Transition the split container to Split+detail-focused.
	var splitCmd tea.Cmd
	m.split, splitCmd = m.split.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	// Sync both panel sizes now that the layout changed.
	m = m.withSyncedListSize()
	// Load the commit into the detail panel.
	var refCmd tea.Cmd
	m.commitDetail, refCmd = m.commitDetail.WithRef(ref)
	m.commitDetail = m.commitDetail.WithPushState(ui.CommitPushState(selected.class, m.branchDiverged))
	m = m.withSyncedDetailSize()
	return m, tea.Batch(splitCmd, refCmd)
}
