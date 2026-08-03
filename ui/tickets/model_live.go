package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ralphloop"
)

// modelLiveEventMsg wraps one ralphloop.LiveEvent read off Model's liveEvents
// channel, mirroring FlatModel's flatLiveEventMsg (flat_live.go). epicName
// identifies which epic's channel produced ev — closed over at
// startLiveTracking's call time (ticket 05), since most LiveEvent kinds don't
// carry EpicName themselves (see ralphloop/live_events.go). ok is false once
// the channel closes, after which no further reads are scheduled.
type modelLiveEventMsg struct {
	epicName string
	event    ralphloop.LiveEvent
	ok       bool
}

// cmdWaitModelLiveEvent reads the next event off events, matching
// cmdWaitLiveEvent's single-blocking-read-per-Cmd pattern.
func cmdWaitModelLiveEvent(epicName string, events <-chan ralphloop.LiveEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		return modelLiveEventMsg{epicName: epicName, event: ev, ok: ok}
	}
}

// applyLiveEvent folds ev into epicName's slice of m.live/m.labelIdentifier,
// sharing applyLiveEvent's fold with FlatModel (flat_live.go) against that
// epic-scoped inner map (ticket 05) rather than a flat one, so two
// concurrently-running epics' same-numbered tickets don't collide. Model
// tracks no transcript tail, so it passes nil.
func (m Model) applyLiveEvent(epicName string, ev ralphloop.LiveEvent) {
	if m.live[epicName] == nil {
		m.live[epicName] = map[string]liveTicketState{}
	}
	if m.labelIdentifier[epicName] == nil {
		m.labelIdentifier[epicName] = map[string]string{}
	}
	applyLiveEvent(m.live[epicName], m.labelIdentifier[epicName], nil, ev)
}

// startLiveTracking arms a channel reader for events against m.implementEpic
// (the epic identity is read at call time, not passed in, so every call site
// that wants to track a different epic sets m.implementEpic immediately
// before calling this — see handleImplementStarted/handleImplementSync).
// Unless one is already in flight for that epic on this Model instance —
// guarding against a tab reactivation (OnPageActivated) spawning a second
// concurrent reader on the same shared channel alongside the one started when
// the run launched. Returns nil if events is nil (registry has no run in
// flight), m.implementEpic is unset, or a reader for that epic is already
// running.
func (m *Model) startLiveTracking(events <-chan ralphloop.LiveEvent) tea.Cmd {
	epicName := m.implementEpic
	if events == nil || epicName == "" {
		return nil
	}
	if _, already := m.liveEvents[epicName]; already {
		return nil
	}
	if m.liveEvents == nil {
		m.liveEvents = map[string]<-chan ralphloop.LiveEvent{}
	}
	m.liveEvents[epicName] = events
	if m.implementingEpics == nil {
		m.implementingEpics = map[string]bool{}
	}
	m.implementingEpics[epicName] = true
	return cmdWaitModelLiveEvent(epicName, events)
}

// clearLiveTracking resets m.implementEpic's live-orchestrator state — kept
// as this zero-arg convenience for call sites/tests that only ever track one
// epic at a time; a poll/sync learning of a *specific* epic's finish (which
// might not be m.implementEpic once more than one epic is tracked, ticket 05)
// goes through clearLiveTrackingFor(epicName) directly so it can't wipe a
// different, still-running epic's live state.
func (m *Model) clearLiveTracking() {
	m.clearLiveTrackingFor(m.implementEpic)
}

// clearLiveTrackingFor removes epicName's live-orchestrator state once its
// run has finished (or once this Model learns of a finish it missed while
// backgrounded — see handleImplementSync), reverting that epic's ticket/epic
// rendering to the normal disk-based view.
func (m *Model) clearLiveTrackingFor(epicName string) {
	delete(m.liveEvents, epicName)
	delete(m.live, epicName)
	delete(m.labelIdentifier, epicName)
	delete(m.implementingEpics, epicName)
	if m.implementEpic == epicName {
		m.implementEpic = ""
	}
}
