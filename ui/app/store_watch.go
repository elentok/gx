package app

import (
	"time"

	tea "charm.land/bubbletea/v2"

	ticketsui "github.com/elentok/gx/ui/tickets"
)

// storePollInterval is slow on purpose: the watch is the fast path and the
// poll only covers events it lost.
const storePollInterval = 30 * time.Second

// storeWatch is the shell's down-mode view of the ticket store: a file watch
// plus a slow poll, running only while the server is down. gen orphans the
// ticks of a watch that has since stopped.
type storeWatch struct {
	stop   func() // non-nil while running
	events <-chan struct{}
	gen    int
}

type (
	// storeWatchStartMsg starts the watch for a shell that has no server client
	// and so is down from the start (Init cannot change the model itself).
	storeWatchStartMsg struct{}
	storePollMsg       struct{ gen int }
	storeChangedMsg    struct{ gen int }
)

func (m Model) startStoreWatch() (Model, tea.Cmd) {
	if m.store.stop != nil {
		return m, nil
	}
	events, stop, err := ticketsui.WatchStore(ticketsui.ScratchDir(m.settings.ActiveWorktreePath))
	if err != nil {
		// The poll alone still keeps the rows fresh.
		events, stop = nil, func() {}
	}
	m.store.gen++
	m.store.stop, m.store.events = stop, events
	cmds := []tea.Cmd{cmdStorePoll(m.store.gen)}
	if events != nil {
		cmds = append(cmds, cmdStoreWait(m.store.gen, events))
	}
	return m, tea.Batch(cmds...)
}

func (m Model) stopStoreWatch() Model {
	if m.store.stop == nil {
		return m
	}
	m.store.stop()
	m.store.stop, m.store.events = nil, nil
	m.store.gen++ // orphans any tick still in flight
	return m
}

// updateStoreWatch handles the watch's messages; ok is false for any other msg.
func (m Model) updateStoreWatch(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case storeWatchStartMsg:
		m, cmd := m.startStoreWatch()
		return m, cmd, true

	case storePollMsg:
		if msg.gen != m.store.gen || m.store.stop == nil {
			return m, nil, true
		}
		m, notify := m.notifyStoreChanged()
		return m, tea.Batch(notify, cmdStorePoll(msg.gen)), true

	case storeChangedMsg:
		if msg.gen != m.store.gen || m.store.stop == nil {
			return m, nil, true
		}
		m, notify := m.notifyStoreChanged()
		return m, tea.Batch(notify, cmdStoreWait(msg.gen, m.store.events)), true
	}
	return m, nil, false
}

// notifyStoreChanged tells the active page the store changed; hidden pages
// reload when activated.
func (m Model) notifyStoreChanged() (Model, tea.Cmd) {
	current := m.activePage()
	if current.model == nil {
		return m, nil
	}
	var cmd tea.Cmd
	current.model, cmd = current.model.Update(ticketsui.StoreChangedMsg{})
	m.setActivePage(current)
	return m, cmd
}

func cmdStorePoll(gen int) tea.Cmd {
	return tea.Tick(storePollInterval, func(time.Time) tea.Msg { return storePollMsg{gen: gen} })
}

// cmdStoreWait blocks until the store changes; a stopped watch closes events,
// which ends the wait quietly.
func cmdStoreWait(gen int, events <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		if _, ok := <-events; !ok {
			return nil
		}
		return storeChangedMsg{gen: gen}
	}
}
