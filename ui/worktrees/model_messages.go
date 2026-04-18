package worktrees

import "github.com/elentok/gx/git"

type clearStatusMsg struct{ gen int }

type pruneRemotesMsg struct{ err error }

type worktreesLoadedMsg struct {
	worktrees []git.Worktree
	err       error
}

type syncStatusMsg struct {
	branch string
	status git.SyncStatus
}

type dirtyStatusMsg struct {
	worktreePath string
	dirty        dirtyState
}

type sidebarDataMsg struct {
	worktreePath  string
	upstream      string // empty if no remote tracking branch found
	headCommit    git.Commit
	aheadCommits  []git.Commit
	behindCommits []git.Commit
	changes       []git.Change
}

type baseStatusMsg struct {
	branch  string
	rebased bool // true if main is an ancestor of branch
}
