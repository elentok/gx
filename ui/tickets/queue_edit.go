package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/terminalrun"
)

// e-prefix chords: edit the selected ticket's underlying file, mirroring the
// Tickets tab's own edit-file chord (model_keys.go) via the shared
// editTicketFile helper (model_runtime.go).
const (
	bindingQueueEditInPlace keys.BindingID = "edit"
	bindingQueueEditHSplit  keys.BindingID = "edit-hsplit"
	bindingQueueEditVSplit  keys.BindingID = "edit-vsplit"
	bindingQueueEditTab     keys.BindingID = "edit-tab"
	bindingQueueCancelChord keys.BindingID = "cancel-chord"
)

// cmdEditSelectedFile opens the selected row's ticket file for editing,
// mirroring the Tickets tab's Model.cmdEditSelectedFile; an epic header opens
// its ticket.md.
func (m QueueModel) cmdEditSelectedFile(splitType terminalrun.SplitType) tea.Cmd {
	file, ok := m.selectedEditTarget()
	if !ok {
		return notify.Warning("nothing selected")
	}
	return editTicketFile(m.worktreeRoot, m.settings, file, splitType, false)
}

// selectedEditTarget is the file the selected row's address resolves to.
func (m QueueModel) selectedEditTarget() (file string, ok bool) {
	if epic, ok := m.selectedQueueEpic(); ok {
		file, err := epicFile(m.settings.TicketStore, epic)
		return file, err == nil
	}
	row, ok := m.selectedQueueRow()
	if !ok {
		return "", false
	}
	file, _ = resolveTicketPath(m.settings.TicketStore, row.ticket.Path)
	return file, true
}

func (m QueueModel) handleEditFileFinished(msg editFileFinishedMsg) (QueueModel, tea.Cmd) {
	return m, editFileFinishedCmd(msg)
}
