package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// The Tickets tab's rows come from the server's event stream while it is
// reachable. While it is down — or the tab has no client at all — the tab reads
// the store, reloading when the app shell (which runs the store watch) says the
// store changed and when the tab is activated.

// readsDisk reports whether the tab reads the store itself: the server is down
// or there is no client at all.
func (s serverBinding) readsDisk() bool {
	return s.api == nil || s.link == tabapi.ServerLinkDown
}

// OnPageActivated catches the tab up with the store: hidden tabs are not told
// about changes made while the server was down.
func (m Model) OnPageActivated() tea.Cmd {
	if !m.server.readsDisk() {
		return nil
	}
	return cmdLoadDisk(m.scratchDir())
}

// cmdLoadDisk reads `.scratch/` directly: the down-mode loader. A missing
// directory reads as zero epics, and an unreadable `.archive` as nothing
// archived.
func cmdLoadDisk(scratchDir string) tea.Cmd {
	return func() tea.Msg {
		epics, err := tickets.Load(scratchDir)
		archivedEpicCount, _ := tickets.CountArchivedEpics(scratchDir)
		return epicsLoadedMsg{epics: epics, err: err, archivedEpicCount: archivedEpicCount}
	}
}
