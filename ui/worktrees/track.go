package worktrees

import (
	"fmt"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/confirm"

	tea "charm.land/bubbletea/v2"
)

type trackResultMsg struct{ err error }

func cmdTrack(repo git.Repo, wt git.Worktree) tea.Cmd {
	return func() tea.Msg {
		remote := git.BranchRemote(repo, wt.Branch)
		return trackResultMsg{err: git.TrackRemote(repo.Root, remote, wt.Branch)}
	}
}

func (m Model) enterTrackConfirm() Model {
	wt := m.cursorWorktree()
	if wt == nil {
		return m
	}
	remote := git.BranchRemote(m.repo, wt.Branch)
	prompt := fmt.Sprintf("Track %s/%s?\n\nThis will run:\n  git branch --set-upstream-to=%s/%s %s",
		remote, wt.Branch, remote, wt.Branch, wt.Branch)
	return m.enterConfirm(confirm.Options{Prompt: prompt, AcceptCmd: cmdTrack(m.repo, *wt), SpinnerLabel: "Tracking " + wt.Name + "…"})
}
