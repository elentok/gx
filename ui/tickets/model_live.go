package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/notify"
)

// closeNotifyCmd batches a notify.Close cmd per id, or nil if ids is empty.
func closeNotifyCmd(ids []string) tea.Cmd {
	if len(ids) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, len(ids))
	for i, id := range ids {
		cmds[i] = notify.Close(id)
	}
	return tea.Batch(cmds...)
}

// toastNotifyCmd batches a notify cmd per queued toast (see
// epicRun.pendingToasts), or nil if toasts is empty.
func toastNotifyCmd(toasts []notify.NotifyMsg) tea.Cmd {
	if len(toasts) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, len(toasts))
	for i, msg := range toasts {
		cmds[i] = func() tea.Msg { return msg }
	}
	return tea.Batch(cmds...)
}
