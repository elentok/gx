package prs

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

type prsLoadedMsg struct {
	prs []git.PR
	err error
}

func (m Model) cmdLoad() tea.Cmd {
	worktreeRoot := m.worktreeRoot
	return func() tea.Msg {
		prs, err := git.ListOpenPRs(worktreeRoot)
		return prsLoadedMsg{prs: prs, err: err}
	}
}

type gotoPRMsg struct {
	url string
}

func (m Model) cmdOpenSelected() tea.Cmd {
	sel := m.list.Selected()
	if sel < 0 || sel >= len(m.prs) {
		return nil
	}
	url := m.prs[sel].URL
	return func() tea.Msg { return gotoPRMsg{url: url} }
}

func (m Model) handleGotoPR(msg gotoPRMsg) (Model, tea.Cmd) {
	if msg.url == "" {
		return m, notify.Warning("no PR URL found")
	}
	return m, ui.CmdOpenURL(msg.url)
}
