package worktrees

import (
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"

	tea "charm.land/bubbletea/v2"
)

// cmdLoadWorktrees also resolves trunk again, so a project.json edit shows up
// on the next reload.
func cmdLoadWorktrees(repo git.Repo, store string) tea.Cmd {
	return func() tea.Msg {
		wts, err := git.ListWorktrees(repo)
		trunk, trunkErr := tickets.TrunkIn(store, repo.Root)
		return worktreesLoadedMsg{worktrees: wts, err: err, trunk: trunk, trunkErr: trunkErr}
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
		rebased, _ := git.IsRebasedOnTrunk(repo, branch)
		return baseStatusMsg{branch: branch, rebased: rebased}
	}
}

func cmdLoadPreviewData(repo git.Repo, wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		upstream := git.UpstreamBranch(repo.Root, wt.Branch)
		var aheadCommits, behindCommits []git.Commit
		if upstream != "" {
			aheadCommits, _ = git.CommitsBetween(repo, upstream, wt.Branch)
			behindCommits, _ = git.CommitsBetween(repo, wt.Branch, upstream)
		}
		var trunkAhead, trunkBehind []git.Commit
		if repo.Trunk != "" && wt.Branch != "" && wt.Branch != repo.Trunk {
			trunkAhead, _ = git.CommitsBetween(repo, repo.Trunk, wt.Branch)
			trunkBehind, _ = git.CommitsBetween(repo, wt.Branch, repo.Trunk)
		}
		headCommit, _ := git.HeadCommit(wt.Path, "HEAD")
		changes, _ := git.UncommittedChanges(wt.Path)
		return previewDataMsg{
			worktreePath:  wt.Path,
			upstream:      upstream,
			headCommit:    headCommit,
			aheadCommits:  aheadCommits,
			behindCommits: behindCommits,
			trunkAhead:    len(trunkAhead),
			trunkBehind:   len(trunkBehind),
			changes:       changes,
		}
	}
}
