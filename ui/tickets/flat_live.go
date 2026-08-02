package tickets

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// liveTicketState is a ticket's in-memory-only orchestrator state — which
// iteration is running/paused and why — layered on top of its on-disk
// Status: line (ticket 01 keeps ticket status itself on disk; only this
// moved onto the event stream). A ticket absent from FlatModel.live has had
// no live event touch it yet, so its row falls back to ticket 03's
// disk-only rendering unchanged.
type liveTicketState struct {
	running   bool
	paused    bool
	label     string // iteration/tab label, e.g. "iter-04a"
	pauseKind ralphloop.PauseKind
	reason    string
	phase     livePhase
}

// livePhase distinguishes what a running iteration is doing right now, so
// renderLiveTicketRow can show a more specific row-suffix than a bare
// spinner+label. Zero value (livePhaseImplementing) covers the whole span
// from LiveEventIterationStarted until the agent's commits are cherry-picked
// — the common case, so it needs no dedicated event to enter.
type livePhase int

const (
	livePhaseImplementing livePhase = iota
	livePhaseCherryPicking
	livePhaseResolvingConflicts
	livePhaseCompacting
	livePhaseFinishingUp
)

// suffix returns p's row-suffix text, e.g. "(implementing...)".
func (p livePhase) suffix() string {
	switch p {
	case livePhaseCherryPicking:
		return "(cherry-picking...)"
	case livePhaseResolvingConflicts:
		return "(resolving conflicts...)"
	case livePhaseCompacting:
		return "(compacting...)"
	case livePhaseFinishingUp:
		return "(telling the agent to finish up...)"
	default:
		return "(implementing...)"
	}
}

// flatTranscriptMaxLines bounds FlatModel.transcript's per-ticket tail so a
// long-running iteration's transcript doesn't grow unbounded in memory —
// only the most recent lines matter for a "live tail" (ticket 04b).
const flatTranscriptMaxLines = 200

// WithLiveEvents wires events as FlatModel's live orchestrator event source.
// Only cmd/ralphloop.go's production wiring calls this (see
// runRalphLoopTUI); a FlatModel built without it (every existing test) keeps
// ticket 03's disk-only rendering, since a nil channel is never read from.
func (m FlatModel) WithLiveEvents(events <-chan ralphloop.LiveEvent) FlatModel {
	m.liveEvents = events
	return m
}

// flatLiveEventMsg wraps one ralphloop.LiveEvent read off FlatModel's
// liveEvents channel for Update. ok is false once the channel closes (the
// orchestrator's Run call returned), after which no further reads are
// scheduled.
type flatLiveEventMsg struct {
	event ralphloop.LiveEvent
	ok    bool
}

// cmdWaitLiveEvent reads the next event off events. FlatModel re-issues this
// after handling each message, the standard bubbletea channel-listen
// pattern — a single blocking read per tea.Cmd invocation.
func cmdWaitLiveEvent(events <-chan ralphloop.LiveEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		return flatLiveEventMsg{event: ev, ok: ok}
	}
}

// applyLiveEvent folds one orchestrator LiveEvent into m.live/
// m.labelIdentifier. live and labelIdentifier are maps (reference types), so
// mutating them through a value receiver is safe — only fields reassigned
// wholesale need a pointer receiver, and this method never does that.
func (m FlatModel) applyLiveEvent(ev ralphloop.LiveEvent) {
	switch ev.Kind {
	case ralphloop.LiveEventIterationStarted, ralphloop.LiveEventTicketReattached:
		m.labelIdentifier[ev.Label] = ev.Identifier
		m.live[ev.Identifier] = liveTicketState{running: true, label: ev.Label}

	case ralphloop.LiveEventIterationPaused:
		if identifier, ok := m.labelIdentifier[ev.Label]; ok {
			m.live[identifier] = liveTicketState{
				paused: true, label: ev.Label, pauseKind: ev.PauseKind, reason: ev.Reason,
			}
		}

	case ralphloop.LiveEventIterationResumed:
		if identifier, ok := m.labelIdentifier[ev.Label]; ok {
			m.live[identifier] = liveTicketState{running: true, label: ev.Label}
		}

	case ralphloop.LiveEventIterationFinished, ralphloop.LiveEventTicketReverted,
		ralphloop.LiveEventTicketCleanupFinished, ralphloop.LiveEventTicketRecovered:
		delete(m.live, ev.Identifier)
		delete(m.transcript, ev.Identifier)

	case ralphloop.LiveEventTranscriptLine:
		if identifier, ok := m.labelIdentifier[ev.Label]; ok {
			lines := append(m.transcript[identifier], ev.Line)
			if len(lines) > flatTranscriptMaxLines {
				lines = lines[len(lines)-flatTranscriptMaxLines:]
			}
			m.transcript[identifier] = lines
		}

	case ralphloop.LiveEventTicketStillNeedsAttention:
		m.live[ev.Identifier] = liveTicketState{
			paused: true, pauseKind: ralphloop.PauseNeedsAttention, reason: "no live iteration to reattach to",
		}

	case ralphloop.LiveEventTicketRecovering:
		m.live[ev.Identifier] = liveTicketState{running: true}

	case ralphloop.LiveEventTicketUnrecoverable:
		m.live[ev.Identifier] = liveTicketState{
			paused: true, pauseKind: ralphloop.PauseNeedsAttention, reason: "commits missing from epic; needs operator review",
		}

	case ralphloop.LiveEventCherryPickStarted:
		if ls, ok := m.live[ev.Identifier]; ok {
			ls.phase = livePhaseCherryPicking
			m.live[ev.Identifier] = ls
		}

	case ralphloop.LiveEventConflictResolutionStarted:
		if ls, ok := m.live[ev.Identifier]; ok {
			ls.phase = livePhaseResolvingConflicts
			m.live[ev.Identifier] = ls
		}

	case ralphloop.LiveEventSmartZoneCompactStarted:
		if ls, ok := m.live[ev.Identifier]; ok {
			ls.phase = livePhaseCompacting
			m.live[ev.Identifier] = ls
		}

	case ralphloop.LiveEventSmartZoneFinishingUp:
		if ls, ok := m.live[ev.Identifier]; ok {
			ls.phase = livePhaseFinishingUp
			m.live[ev.Identifier] = ls
		}

	case ralphloop.LiveEventSmartZoneRecovered:
		if ls, ok := m.live[ev.Identifier]; ok {
			ls.phase = livePhaseImplementing
			m.live[ev.Identifier] = ls
		}
	}
}

// liveStateForSelected returns the selected ticket's live orchestrator state,
// ok only when it's running, paused, or needs-attention — the three states
// ticket 04b's preview pane grows a metadata line/transcript tail for. A
// done/open ticket (no live entry, or one already deleted on finish) reports
// ok=false so the preview stays ticket 03's disk-only shape.
func (m FlatModel) liveStateForSelected() (liveTicketState, bool) {
	t, ok := m.selectedTicket()
	if !ok {
		return liveTicketState{}, false
	}
	live, ok := m.live[t.Identifier]
	if !ok || (!live.running && !live.paused) {
		return liveTicketState{}, false
	}
	return live, true
}

// WithTabFocus overrides the herdr.TabFocus call FlatModel makes on `enter`
// (ticket 05). Only tests use this, to avoid shelling out to a real herdr
// binary; production wiring (cmd/ralphloop.go) leaves NewFlatModel's default
// (herdr.TabFocus) in place.
func (m FlatModel) WithTabFocus(fn func(tabID string) (herdr.Tab, error)) FlatModel {
	m.tabFocus = fn
	return m
}

// flatTabFocusResultMsg carries the result of a cmdFocusTab call. err is
// non-nil when herdr.TabFocus failed (e.g. tab_not_found because the tab
// closed between the row rendering its label and enter being pressed); the
// only thing FlatModel does with it is surface it as a toast, not treat it as
// fatal.
type flatTabFocusResultMsg struct {
	err error
}

// cmdFocusTab calls m.tabFocus(tabID), jumping the herdr session to that
// ticket's live pane (ticket 05's `enter` binding).
func (m FlatModel) cmdFocusTab(tabID string) tea.Cmd {
	focus := m.tabFocus
	return func() tea.Msg {
		_, err := focus(tabID)
		return flatTabFocusResultMsg{err: err}
	}
}

// liveTabID returns identifier's live herdr tab id (its label) and whether
// one exists: identifier must have a running or paused liveTicketState with
// a non-empty label, which excludes the needs-attention pause kinds that
// have no live iteration to reattach to (see applyLiveEvent).
func (m FlatModel) liveTabID(identifier string) (string, bool) {
	ls, ok := m.live[identifier]
	if !ok || !(ls.running || ls.paused) || ls.label == "" {
		return "", false
	}
	return ls.label, true
}

// hasRunningLiveTicket reports whether any ticket currently has a running
// live iteration, gating whether the shared row spinner keeps ticking.
func (m FlatModel) hasRunningLiveTicket() bool {
	for _, ls := range m.live {
		if ls.running {
			return true
		}
	}
	return false
}

// appendBlockedBySuffix appends suffix to line in blockedBySuffixStyle,
// shared by renderLiveTicketRow's three branches and previewLiveMetaLine
// (flat_view.go) — both append a live ticket's label/reason the same way,
// just onto different base lines.
func appendBlockedBySuffix(line, suffix string) string {
	if suffix == "" {
		return line
	}
	return line + " " + blockedBySuffixStyle.Render(suffix)
}

// renderLiveTicketRow renders t's row from its live orchestrator state
// instead of ticket 03's disk-only rendering, when live is running or
// paused. ok is false if live has neither (the zero value, which shouldn't
// reach here since callers only look live up on a present map entry, but
// keeps this function total).
func (m FlatModel) renderLiveTicketRow(t tickets.Ticket, live liveTicketState) (string, bool) {
	title := fmt.Sprintf("%s %s", t.DisplayNumber(), t.Title)

	switch {
	case live.paused && live.pauseKind == ralphloop.PauseNeedsAttention:
		line := "  " + statusNeedsAttentionStyle.Render(m.icons().TicketNeedsAttention) + " " + title
		return appendBlockedBySuffix(line, live.reason), true

	case live.paused:
		line := "  " + statusPausedStyle.Render(m.icons().TicketPaused) + " " + title
		return appendBlockedBySuffix(line, live.reason), true

	case live.running:
		line := "  " + m.spinner.View() + " " + title
		suffix := live.phase.suffix()
		if live.label != "" {
			suffix = live.label + " " + suffix
		}
		return appendBlockedBySuffix(line, suffix), true

	default:
		return "", false
	}
}
