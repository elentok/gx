package tickets

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
)

// The Tickets tab's rows come from the server's event stream while it is
// reachable. While it is down — or the tab has no client at all — the tab reads
// the store, reloading when the app shell (which runs the store watch) says the
// store changed and when the tab is activated.

// StoreChangedMsg tells the tab the ticket store changed on disk; the app shell
// sends it while the server is down.
type StoreChangedMsg struct{}

// storeWatchDebounce turns a burst of writes into one signal.
const storeWatchDebounce = 50 * time.Millisecond

// readsDisk reports whether the tab reads the store itself: the server is down
// or there is no client at all.
func (m Model) readsDisk() bool {
	return m.serverAPI == nil || m.serverLink == ServerLinkDown
}

// updateServerLink handles the connection events; ok is false for any other msg.
func (m Model) updateServerLink(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg.(type) {
	case ServerDownMsg:
		m.serverLink = ServerLinkDown
		return m, m.cmdLoadDisk(), true

	case ServerUpMsg:
		if m.serverLink == ServerLinkDown {
			m.serverLink = ServerLinkUp
		}
		// The shell re-snapshots on the same probe and delivers the result.
		return m, nil, true

	case StoreChangedMsg:
		if !m.readsDisk() {
			return m, nil, true
		}
		return m, m.cmdLoadDisk(), true
	}
	return m, nil, false
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

// ScratchDir is the ticket store directory for worktreeRoot's repo.
func ScratchDir(worktreeRoot string) string { return scratchDirFor(worktreeRoot) }

// WatchStore signals (debounced) on every change under dir. stop ends the
// watch and closes the channel.
func WatchStore(dir string) (events <-chan struct{}, stop func(), err error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}
	server.WatchTree(w, dir)
	out := make(chan struct{}, 1)
	go func() {
		defer close(out)
		var debounce <-chan time.Time
		for {
			select {
			case _, ok := <-w.Events:
				if !ok {
					return
				}
				if debounce == nil {
					debounce = time.After(storeWatchDebounce)
				}
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			case <-debounce:
				debounce = nil
				server.WatchTree(w, dir) // pick up directories created since
				select {
				case out <- struct{}{}:
				default: // a signal is already pending
				}
			}
		}
	}()
	return out, func() { _ = w.Close() }, nil
}
