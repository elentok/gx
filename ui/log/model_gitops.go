package log

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/gitops"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

// handleGitopsUpdate routes msg to the open bump/push/pull modal. Unlike
// status, log reloads the list even when push or pull fail or are aborted,
// shows a bump error as a toast, and announces RepoMutated on success.
func (m Model) handleGitopsUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, res := m.git.Update(msg)
	m.git = next
	if !res.Done {
		return m, cmd
	}
	succeeded := res.Err == nil && !res.Aborted
	switch res.Kind {
	case gitops.KindBump:
		switch {
		case res.Err != nil && res.NewTag == "":
			return m, notify.Error("bump failed: " + res.Err.Error())
		case res.Err != nil:
			return m, notify.Error(res.Err.Error())
		case res.NewTag == "":
			return m, nil
		}
		return m, tea.Batch(cmd, nav.RepoMutated())
	case gitops.KindPush, gitops.KindPull:
		if succeeded {
			return m, tea.Batch(cmd, m.cmdReload(), nav.RepoMutated())
		}
		return m, tea.Batch(cmd, m.cmdReload())
	}
	return m, cmd
}
