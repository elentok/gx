package commit

import (
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/commitops"
	"github.com/elentok/gx/ui/nav"
)

// IsModalActive reports whether the detail panel is driving its own modal
// (amend or reword) that runs asynchronous commands. Containers that embed the
// panel must keep forwarding every message to it while this is true; otherwise
// the modal's async step/spinner messages are dropped and it stalls mid-run.
func (m Model) IsModalActive() bool {
	return m.commitOps.IsOpen()
}

func (m Model) handleCommitOpsUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.commitOps.Update(m.worktreeRoot, msg)
	m.commitOps = next
	if !result.Done || result.Err != nil {
		return m, cmd
	}
	focus := m.details.Subject
	if result.Kind == commitops.KindReword {
		focus = result.NewSubject
	}
	return m, nav.Switch(nav.ViewState{
		Tab:          nav.TabLog,
		WorktreeRoot: m.worktreeRoot,
		Ref:          "HEAD",
		FocusSubject: focus,
	})
}
