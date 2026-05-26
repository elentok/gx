package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"
)

type gotoPRMsg struct {
	url      string
	err      error
	fatalErr error // non-nil means show as error (from IsCommitMergedToMain)
}

func (m Model) cmdGotoPR() tea.Cmd {
	worktreeRoot := m.worktreeRoot
	ref := m.ref
	return func() tea.Msg {
		merged, err := git.IsCommitMergedToMain(worktreeRoot, ref)
		if err != nil {
			return gotoPRMsg{fatalErr: err}
		}
		var url string
		if merged {
			url, err = git.CommitPRURL(worktreeRoot, ref)
		} else {
			url, err = git.BranchPRURL(worktreeRoot)
		}
		return gotoPRMsg{url: url, err: err}
	}
}

func (m Model) handleGotoPR(msg gotoPRMsg) (tea.Model, tea.Cmd) {
	if msg.fatalErr != nil {
		return m, notify.Error(msg.fatalErr.Error())
	}
	if msg.err != nil || msg.url == "" {
		return m, notify.Warning("no PR found")
	}
	return m, ui.CmdOpenURL(msg.url)
}
