package commit

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
)

func (m *Model) openAmendConfirm() error {
	if m.details.FullHash == "" {
		return fmt.Errorf("no commit loaded")
	}
	return m.amendConfirm.Open(m.worktreeRoot, m.details.FullHash, m.details.Subject)
}

// IsModalActive reports whether the detail panel is driving its own modal
// (amend or reword) that runs asynchronous commands. Containers that embed the
// panel must keep forwarding every message to it while this is true; otherwise
// the modal's async step/spinner messages are dropped and it stalls mid-run.
func (m Model) IsModalActive() bool {
	return m.amendConfirm.IsOpen || m.reword.IsOpen
}

func (m Model) handleAmendUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd, result := m.amendConfirm.Update(msg)
	m.amendConfirm = next
	if result.Done {
		return m.handleAmendDone(result.Err)
	}
	return m, cmd
}

func (m Model) handleAmendDone(err error) (tea.Model, tea.Cmd) {
	if err != nil {
		return m, notify.Error("Amend failed: " + err.Error())
	}
	return m, nav.Switch(nav.ViewState{
		Tab:          nav.TabLog,
		WorktreeRoot: m.worktreeRoot,
		Ref:          "HEAD",
		FocusSubject: m.details.Subject,
	})
}
