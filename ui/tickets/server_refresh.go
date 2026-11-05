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
func (m Model) readsDisk() bool {
	return m.serverAPI == nil || m.link == tabapi.ServerLinkDown
}

// updateStoreChanged handles StoreChangedMsg; ok is false for any other msg.
func (m Model) updateStoreChanged(msg tea.Msg) (Model, tea.Cmd, bool) {
	if _, isChange := msg.(tabapi.StoreChangedMsg); !isChange {
		return m, nil, false
	}
	if !m.readsDisk() {
		return m, nil, true
	}
	return m, m.cmdLoadDisk(), true
}

// OnPageActivated catches the tab up with the store: hidden tabs are not told
// about changes made while the server was down.
func (m Model) OnPageActivated() tea.Cmd {
	if !m.readsDisk() {
		return nil
	}
	return m.cmdLoadDisk()
}

// cmdLoadDisk reads `.scratch/` directly: the down-mode loader. A missing
// directory reads as zero epics, and an unreadable `.archive` as nothing
// archived.
func (m Model) cmdLoadDisk() tea.Cmd {
	scratchDir := m.scratchDir()
	return func() tea.Msg {
		epics, err := tickets.Load(scratchDir)
		archivedEpicCount, _ := tickets.CountArchivedEpics(scratchDir)
		return epicsLoadedMsg{epics: epics, err: err, archivedEpicCount: archivedEpicCount}
	}
}
