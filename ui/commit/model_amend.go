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
	return m, nav.Replace(nav.Route{
		Kind:         nav.RouteLog,
		WorktreeRoot: m.worktreeRoot,
		Ref:          "HEAD",
		FocusSubject: m.details.Subject,
	})
}
