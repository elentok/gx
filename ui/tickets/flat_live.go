package tickets

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

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
}

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

	case ralphloop.LiveEventTicketStillNeedsAttention:
		m.live[ev.Identifier] = liveTicketState{
			paused: true, pauseKind: ralphloop.PauseNeedsAttention, reason: "no live iteration to reattach to",
		}

	case ralphloop.LiveEventTicketUnrecoverable:
		m.live[ev.Identifier] = liveTicketState{
			paused: true, pauseKind: ralphloop.PauseNeedsAttention, reason: "commits missing from epic; needs operator review",
		}
	}
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
		if live.reason != "" {
			line += " " + blockedBySuffixStyle.Render(live.reason)
		}
		return line, true

	case live.paused:
		line := "  " + statusPausedStyle.Render(m.icons().TicketPaused) + " " + title
		if live.reason != "" {
			line += " " + blockedBySuffixStyle.Render(live.reason)
		}
		return line, true

	case live.running:
		line := "  " + m.spinner.View() + " " + title
		if live.label != "" {
			line += " " + blockedBySuffixStyle.Render(live.label)
		}
		return line, true

	default:
		return "", false
	}
}
