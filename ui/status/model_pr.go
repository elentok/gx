package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

type gotoPRMsg struct {
	url string
	err error
}

func cmdGotoPR(worktreeRoot string) tea.Cmd {
	return func() tea.Msg {
		url, err := pr.BranchPRURL(worktreeRoot)
		return gotoPRMsg{url: url, err: err}
	}
}

func handleGotoPR(m Model, msg gotoPRMsg) (Model, tea.Cmd) {
	if msg.err != nil || msg.url == "" {
		return m, notify.Warning("no PR found")
	}
	return m, ui.CmdOpenURL(msg.url)
}
