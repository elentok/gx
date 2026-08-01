package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ralphloop"
)

// modelLiveEventMsg wraps one ralphloop.LiveEvent read off Model's liveEvents
// channel, mirroring FlatModel's flatLiveEventMsg (flat_live.go). ok is false
// once the channel closes, after which no further reads are scheduled.
type modelLiveEventMsg struct {
	event ralphloop.LiveEvent
	ok    bool
}

// cmdWaitModelLiveEvent reads the next event off events, matching
// cmdWaitLiveEvent's single-blocking-read-per-Cmd pattern.
func cmdWaitModelLiveEvent(events <-chan ralphloop.LiveEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		return modelLiveEventMsg{event: ev, ok: ok}
	}
}

// applyLiveEvent folds ev into m.live/m.labelIdentifier, sharing
// applyLiveEvent's fold with FlatModel (flat_live.go). Model tracks no
// transcript tail, so it passes nil.
func (m Model) applyLiveEvent(ev ralphloop.LiveEvent) {
	applyLiveEvent(m.live, m.labelIdentifier, nil, ev)
}

// startLiveTracking arms a channel reader for events, unless one is already
// in flight for this Model instance (m.liveEvents != nil) — guarding against
// a tab reactivation (OnPageActivated) spawning a second concurrent reader on
// the same shared channel alongside the one started when the run launched.
// Returns nil if events is nil (registry has no run in flight) or a reader is
// already running.
func (m *Model) startLiveTracking(events <-chan ralphloop.LiveEvent) tea.Cmd {
	if events == nil || m.liveEvents != nil {
		return nil
	}
	m.liveEvents = events
	return cmdWaitModelLiveEvent(events)
}

// clearLiveTracking resets this Model's live-orchestrator state once its run
// has finished (or once this Model learns of a finish it missed while
// backgrounded — see handleImplementSync), reverting ticket/epic rendering to
// the normal disk-based view.
func (m *Model) clearLiveTracking() {
	m.implementEpic = ""
	m.liveEvents = nil
	m.live = map[string]liveTicketState{}
	m.labelIdentifier = map[string]string{}
}
