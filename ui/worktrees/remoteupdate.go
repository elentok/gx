package worktrees

import (
	"github.com/elentok/gx/git"

	tea "charm.land/bubbletea/v2"
)

type remoteUpdateResultMsg struct {
	err error
	log string
}

func cmdRemoteUpdate(repo git.Repo) tea.Cmd {
	return func() tea.Msg {
		out, err := git.UpdateRemotes(repo)
		return remoteUpdateResultMsg{err: err, log: out}
	}
}
