package worktrees

import (
	"time"

	"gx/git"

	tea "charm.land/bubbletea/v2"
)

func cmdClearStatus(gen int) tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsg{gen: gen}
	})
}

func cmdLoadWorktrees(repo git.Repo) tea.Cmd {
	return func() tea.Msg {
		wts, err := git.ListWorktrees(repo)
		return worktreesLoadedMsg{worktrees: wts, err: err}
	}
}

func cmdPruneRemotes(repo git.Repo) tea.Cmd {
	return func() tea.Msg {
		err := git.PruneAllRemotes(repo)
		return pruneRemotesMsg{err: err}
	}
}

func cmdLoadSyncStatus(repo git.Repo, branch string) tea.Cmd {
	return func() tea.Msg {
		status, _ := git.WorktreeSyncStatus(repo, branch)
		return syncStatusMsg{branch: branch, status: status}
	}
}

func cmdLoadDirtyStatus(wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		changes, _ := git.UncommittedChanges(wt.Path)
		return dirtyStatusMsg{
			worktreePath: wt.Path,
			dirty:        dirtyStateFromChanges(changes),
		}
	}
}

func cmdLoadBaseStatus(repo git.Repo, branch string) tea.Cmd {
	return func() tea.Msg {
		rebased, _ := git.IsRebasedOnMain(repo, branch)
		return baseStatusMsg{branch: branch, rebased: rebased}
	}
}

func cmdLoadSidebarData(repo git.Repo, wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		upstream := git.UpstreamBranch(repo.Root, wt.Branch)
		var aheadCommits, behindCommits []git.Commit
		if upstream != "" {
			aheadCommits, _ = git.CommitsBetween(repo, upstream, wt.Branch)
			behindCommits, _ = git.CommitsBetween(repo, wt.Branch, upstream)
		}
		headCommit, _ := git.HeadCommit(wt.Path, "HEAD")
		changes, _ := git.UncommittedChanges(wt.Path)
		return sidebarDataMsg{
			worktreePath:  wt.Path,
			upstream:      upstream,
			headCommit:    headCommit,
			aheadCommits:  aheadCommits,
			behindCommits: behindCommits,
			changes:       changes,
		}
	}
}
