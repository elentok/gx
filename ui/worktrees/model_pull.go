package worktrees

import (
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

func (m Model) handlePullUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.pull.Update(msg)
	m.pull = next
	if !result.Done {
		return m, cmd
	}

	wt := m.pullWT
	m.pullWT = nil
	m.lastJobLog = result.Output
	m.lastJobLabel = "Pull output"

	if result.Err != nil {
		return m.showError(result.Err.Error()), nil
	}

	cmds := []tea.Cmd{notify.Info(ui.MessageComplete("pull"))}
	if wt != nil && wt.Branch != "" {
		cmds = append(cmds, cmdLoadSyncStatus(m.repo, wt.Branch), cmdLoadPreviewData(m.repo, *wt))
		if wt.Branch == m.repo.MainBranch {
			for _, w := range m.worktrees {
				if w.Branch != "" {
					cmds = append(cmds, cmdLoadBaseStatus(m.repo, w.Branch))
				}
			}
		}
	}
	return m, tea.Batch(cmds...)
}
