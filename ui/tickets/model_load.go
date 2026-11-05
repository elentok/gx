package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

type epicsLoadedMsg struct {
	epics             []tickets.Epic
	err               error
	archivedEpicCount int
}

// cmdLoad reloads from wherever the rows currently come from: the server's
// snapshot, or the store itself in down mode.
func (m Model) cmdLoad() tea.Cmd {
	if m.server.ReadsDisk() {
		return cmdLoadDisk(m.scratchDir())
	}
	return func() tea.Msg { return tabapi.ResnapshotRequestedMsg{} }
}

// cmdRefresh reloads .scratch/ from disk, matching every other tab's manual
// refresh convention (`R`): a success notification alongside the reload.
func (m Model) cmdRefresh() tea.Cmd {
	return tea.Batch(notify.Success("refreshed"), m.cmdLoad())
}
