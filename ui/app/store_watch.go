package app

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// storePollInterval is slow on purpose: the watch is the fast path and the
// poll only covers events it lost.
const storePollInterval = 30 * time.Second

// storeWatch is the shell's down-mode view of the ticket store: a file watch
// plus a slow poll, running only while the server is down. gen orphans the
// ticks of a watch that has since stopped.
type storeWatch struct {
	running bool
	// stop and events are nil when the file watcher failed to start; the poll
	// alone then keeps running.
	stop   func()
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

// start watches dir (the ticket scratch directory); a no-op while running.
func (w *storeWatch) start(dir string) tea.Cmd {
	if w.running {
		return nil
	}
	events, stop, err := tabapi.WatchStore(dir)
	if err != nil {
		// The poll alone still keeps the rows fresh.
		events, stop = nil, nil
	}
	w.gen++
	w.running, w.stop, w.events = true, stop, events
	cmds := []tea.Cmd{cmdStorePoll(w.gen)}
	if events != nil {
		cmds = append(cmds, cmdStoreWait(w.gen, events))
	}
	return tea.Batch(cmds...)
}

func (w *storeWatch) halt() {
	if !w.running {
		return
	}
	if w.stop != nil {
		w.stop()
	}
	w.running, w.stop, w.events = false, nil, nil
	w.gen++ // orphans any tick still in flight
}

// update handles the watch's messages; ok is false for any other msg. changed
// is true when the store may have changed, so the caller must tell the active
// page; next is the command that keeps the watch going.
func (w *storeWatch) update(msg tea.Msg, dir string) (next tea.Cmd, changed, ok bool) {
	switch msg := msg.(type) {
	case storeWatchStartMsg:
		return w.start(dir), false, true

	case storePollMsg:
		if msg.gen != w.gen || !w.running {
			return nil, false, true
		}
		return cmdStorePoll(msg.gen), true, true

	case storeChangedMsg:
		if msg.gen != w.gen || !w.running {
			return nil, false, true
		}
		return cmdStoreWait(msg.gen, w.events), true, true
	}
	return nil, false, false
}

func (m Model) storeDir() string {
	return tabapi.ScratchDir(m.settings.TicketStore, m.settings.ActiveWorktreePath)
}

// updateStoreWatch routes the watch's messages and tells the active page when
// the store changed.
func (m Model) updateStoreWatch(msg tea.Msg) (Model, tea.Cmd, bool) {
	next, changed, ok := m.store.update(msg, m.storeDir())
	if !changed {
		return m, next, ok
	}
	m, notify := m.notifyStoreChanged()
	return m, tea.Batch(notify, next), true
}

// notifyStoreChanged tells the active page the store changed; hidden pages
// reload when activated.
func (m Model) notifyStoreChanged() (Model, tea.Cmd) {
	current := m.activePage()
	if current.model == nil {
		return m, nil
	}
	var cmd tea.Cmd
	current.model, cmd = current.model.Update(tabapi.StoreChangedMsg{})
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
