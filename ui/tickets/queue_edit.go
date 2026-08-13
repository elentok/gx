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
// mirroring the Tickets tab's Model.cmdEditSelectedFile — the Queue tab only
// ever selects tickets (no epic rows), so there's no map.md case to handle.
func (m QueueModel) cmdEditSelectedFile(splitType terminalrun.SplitType) tea.Cmd {
	row, ok := m.selectedQueueRow()
	if !ok {
		return notify.Warning("nothing selected")
	}
	return editTicketFile(m.worktreeRoot, m.settings, row.ticket.Path, splitType)
}

func (m QueueModel) handleEditFileFinished(msg editFileFinishedMsg) (QueueModel, tea.Cmd) {
	return m, editFileFinishedCmd(msg)
}
