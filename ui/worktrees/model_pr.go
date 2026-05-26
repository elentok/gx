package worktrees

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

type gotoPRMsg struct {
	url string
	err error
}

func (m Model) cmdGotoPR(worktreeRoot string) tea.Cmd {
	return func() tea.Msg {
		url, err := git.BranchPRURL(worktreeRoot)
		return gotoPRMsg{url: url, err: err}
	}
}

func (m Model) handleGotoPR(msg gotoPRMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil || msg.url == "" {
		return m, notify.Warning("no PR found")
	}
	return m, ui.CmdOpenURL(msg.url)
}
