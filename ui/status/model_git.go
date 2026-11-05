package status

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/gitops"
)

// handleGitUpdate routes a message to the open bump/push/pull/stash modal and
// reacts to its result: a bump error opens the git-error modal, a finished
// push, pull or stash refreshes and announces the repo mutation.
func (m Model) handleGitUpdate(msg tea.Msg) (Model, tea.Cmd) {
	next, cmd, result := m.git.Update(msg)
	m.git = next
	if result.Kind == gitops.KindBump && result.Err != nil {
		m.showGitError(result.Err)
		return m, nil
	}
	if !result.Mutated() {
		return m, cmd
	}
	return m, tea.Batch(cmd, m.refreshPreserveScroll(), statusRepoMutatedCmd())
}
