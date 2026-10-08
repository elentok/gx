package tickets

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
)

// The Tickets tab's rows come from the server's event stream (no watch, no
// timers) while it is reachable. While it is down — or the tab has no client
// at all — the tab reads the store itself: a watch plus a slow poll.
const (
	// fallbackPollInterval is slow on purpose: the watch is the fast path and
	// the poll only covers events it lost.
	fallbackPollInterval = 30 * time.Second
	// A burst of writes becomes one reload.
	fallbackWatchDebounce = 50 * time.Millisecond
)

type (
	fallbackPollMsg    struct{ gen int }
	fallbackChangedMsg struct{ gen int }
)

// onFallback reports whether the tab is reading the store itself.
func (m Model) onFallback() bool { return m.fallbackStop != nil }

// updateServerLink handles the connection events; ok is false for any other msg.
func (m Model) updateServerLink(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case ServerDownMsg:
		if m.onFallback() {
			return m, nil, true
		}
		m.serverLink = ServerLinkDown
		events, stop, err := watchStore(m.scratchDir())
		if err != nil {
			// The poll alone still keeps the rows fresh.
			events, stop = nil, func() {}
		}
		m.fallbackGen++
		m.fallbackStop = stop
		cmds := []tea.Cmd{m.cmdLoadDisk(), cmdFallbackPoll(m.fallbackGen)}
		if events != nil {
			cmds = append(cmds, cmdFallbackWait(m.fallbackGen, events))
			m.fallbackEvents = events
		}
		return m, tea.Batch(cmds...), true

	case ServerUpMsg:
		if !m.onFallback() || m.serverAPI == nil {
			return m, nil, true
		}
		m.fallbackStop()
		m.fallbackStop, m.fallbackEvents = nil, nil
		m.fallbackGen++ // orphans any tick still in flight
		if m.serverLink == ServerLinkDown {
			m.serverLink = ServerLinkUp
		}
		return m, m.cmdServerSnapshot(), true

	case fallbackPollMsg:
		if msg.gen != m.fallbackGen || m.fallbackStop == nil {
			return m, nil, true
		}
		return m, tea.Batch(m.cmdLoadDisk(), cmdFallbackPoll(msg.gen)), true

	case fallbackChangedMsg:
		if msg.gen != m.fallbackGen || m.fallbackStop == nil {
			return m, nil, true
		}
		return m, tea.Batch(m.cmdLoadDisk(), cmdFallbackWait(msg.gen, m.fallbackEvents)), true
	}
	return m, nil, false
}

func cmdFallbackPoll(gen int) tea.Cmd {
	return tea.Tick(fallbackPollInterval, func(time.Time) tea.Msg { return fallbackPollMsg{gen: gen} })
}

// cmdFallbackWait blocks until the store changes; a stopped watch closes
// events, which ends the wait quietly.
func cmdFallbackWait(gen int, events <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		if _, ok := <-events; !ok {
			return nil
		}
		return fallbackChangedMsg{gen: gen}
	}
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

// watchStore signals (debounced) on every change under dir. stop ends the
// watch and closes the channel.
func watchStore(dir string) (events <-chan struct{}, stop func(), err error) {
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
					debounce = time.After(fallbackWatchDebounce)
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
