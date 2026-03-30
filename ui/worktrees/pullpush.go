package worktrees

import (
	"fmt"

	"gx/git"
	"gx/ui"

	tea "charm.land/bubbletea/v2"
)

type pullResultMsg struct {
	err error
	log string
}
type pushResultMsg struct {
	err   error
	prURL string
	log   string
}
type forcePushResultMsg struct {
	err error
	log string
}

type stashPullResultMsg struct {
	err     error
	log     string
	stashed bool
	wtPath  string
}

type rebasePreflightMsg struct {
	repo git.Repo
	wt   git.Worktree
}

type rebaseResultMsg struct {
	err     error
	log     string
	stashed bool
	wtPath  string
}

type stashPopResultMsg struct {
	err     error
	opLabel string
}

func cmdPull(wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		out, err := git.Pull(wt.Path)
		return pullResultMsg{err: err, log: out}
	}
}

func cmdPush(repo git.Repo, wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		remote := git.BranchRemote(repo, wt.Branch)
		prURL, out, err := git.PushBranch(wt.Path, remote, wt.Branch)
		return pushResultMsg{err: err, prURL: prURL, log: out}
	}
}

func cmdForcePush(repo git.Repo, wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		remote := git.BranchRemote(repo, wt.Branch)
		out, err := git.PushBranchForce(wt.Path, remote, wt.Branch)
		return forcePushResultMsg{err: err, log: out}
	}
}

func cmdOpenURL(url string) tea.Cmd {
	return ui.CmdOpenURL(url)
}

func cmdStashPull(wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		if _, err := git.Stash(wt.Path); err != nil {
			return stashPullResultMsg{err: fmt.Errorf("stash failed: %w", err), wtPath: wt.Path}
		}
		out, err := git.Pull(wt.Path)
		return stashPullResultMsg{err: err, log: out, stashed: true, wtPath: wt.Path}
	}
}

func cmdRebasePreflight(repo git.Repo, wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		return rebasePreflightMsg{repo: repo, wt: wt}
	}
}

func cmdRebase(repo git.Repo, wt git.Worktree, stash bool) tea.Cmd {
	return func() tea.Msg {
		stashed := false
		if stash {
			if _, err := git.Stash(wt.Path); err != nil {
				return rebaseResultMsg{err: fmt.Errorf("stash failed: %w", err), wtPath: wt.Path}
			}
			stashed = true
		}
		out, err := git.Rebase(wt.Path, repo.MainBranch)
		return rebaseResultMsg{err: err, log: out, stashed: stashed, wtPath: wt.Path}
	}
}

func cmdStashPop(wtPath, opLabel string) tea.Cmd {
	return func() tea.Msg {
		_, err := git.StashPop(wtPath)
		return stashPopResultMsg{err: err, opLabel: opLabel}
	}
}

func forcePushPrompt(wt git.Worktree) string {
	return fmt.Sprintf("Push rejected for %s. Force push?", wt.Branch)
}
