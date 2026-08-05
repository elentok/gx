package tickets

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

// liveTicketState is a ticket's in-memory-only orchestrator state — which
// iteration is running/paused and why — layered on top of its on-disk
// Status: line (ticket 01 keeps ticket status itself on disk; only this
// lives in the registry's run snapshot). A ticket absent from Model.live has
// had no snapshot touch it yet, so its row falls back to ticket 03's
// disk-only rendering unchanged.
type liveTicketState struct {
	running   bool
	paused    bool
	label     string // iteration/tab label, e.g. "iter-04a"
	pauseKind ralphloop.PauseKind
	reason    string
	phase     livePhase
	// startedAt is this ticket's own start time (RunTicketSnapshot.StartedAt),
	// stamped when its iteration begins, so elapsed time keeps climbing across
	// a pause/resume instead of resetting, and doesn't conflate two tickets in
	// the same run. Zero if the ticket hasn't started.
	startedAt time.Time
	// tokens is the last context-occupancy reading for this ticket —
	// frozen (not zeroed) while paused, since the underlying session's
	// context hasn't changed.
	tokens int
}

// projectLiveTickets turns a run snapshot into per-ticket live state
// (running/paused, phase, tokens, startedAt) — the one projection both tabs'
// syncRunSnapshot methods call, so a field added to RunTicketSnapshot only
// needs wiring into liveTicketState once. Ticket 21: this used to be two
// hand-duplicated copies (Model.syncRunSnapshot, QueueModel.syncRunSnapshot)
// that had already drifted out of sync once.
func projectLiveTickets(snapshot RunSnapshot) map[string]liveTicketState {
	live := make(map[string]liveTicketState, len(snapshot.Tickets))
	for identifier, ticket := range snapshot.Tickets {
		live[identifier] = liveTicketState{
			running:   ticket.Running,
			paused:    ticket.Paused,
			label:     ticket.Label,
			pauseKind: ticket.PauseKind,
			reason:    ticket.PauseReason,
			phase:     livePhaseImplementing,
			tokens:    ticket.ContextTokens,
			startedAt: ticket.StartedAt,
		}
	}
	return live
}

// liveElapsedSeconds computes a running/paused ticket's elapsed time from
// live.startedAt (that ticket's own start time), so it keeps climbing across
// renders without a UI-side stopwatch. Zero if startedAt hasn't been set.
func liveElapsedSeconds(live liveTicketState) int {
	if live.startedAt.IsZero() {
		return 0
	}
	return int(time.Since(live.startedAt).Seconds())
}

// livePhase distinguishes what a running iteration is doing right now, so
// renderLiveTicketRow can show a more specific row-suffix than a bare
// spinner+label. Zero value (livePhaseImplementing) covers the whole span
// from start until the agent's commits are cherry-picked — the common case,
// so it needs no dedicated event to enter.
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

// color returns p's accent color, so a running row's spinner distinguishes
// what the iteration is doing right now at a glance rather than relying
// solely on the text suffix (which reads as the same dim italic for every
// phase).
func (p livePhase) color() color.Color {
	switch p {
	case livePhaseCherryPicking:
		return ui.ColorTeal
	case livePhaseResolvingConflicts:
		return ui.ColorRed
	case livePhaseCompacting:
		return ui.ColorYellow
	case livePhaseFinishingUp:
		return ui.ColorOrange
	default:
		return ui.ColorBlue
	}
}

// appendBlockedBySuffix appends suffix to line in blockedBySuffixStyle, used
// by view.go to attach a running/paused ticket's label or pause reason onto
// its base row.
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
// keeps this function total). base is the bare icon/spinner+title; suffix is
// the phase/label text (running) or pause reason (paused/needs-attention),
// returned separately so the caller can place it where it needs to.
func renderLiveTicketRow(icons ui.IconSet, sp spinner.Model, t tickets.Ticket, live liveTicketState) (base, suffix string, ok bool) {
	title := fmt.Sprintf("%s %s", t.DisplayNumber(), t.Title)

	switch {
	case live.paused && live.pauseKind == ralphloop.PauseNeedsAttention:
		base = "  " + statusNeedsAttentionStyle.Render(icons.TicketNeedsAttention) + " " + title
		return base, live.reason, true

	case live.paused:
		base = "  " + statusPausedStyle.Render(icons.TicketPaused) + " " + title
		return base, live.reason, true

	case live.running:
		// spinner.Dot's frames each carry a trailing space; strip it so the
		// icon column stays single-width and aligned with the other status glyphs.
		spinnerView := lipgloss.NewStyle().Foreground(live.phase.color()).Render(strings.TrimRight(sp.View(), " "))
		base = "  " + spinnerView + " " + title
		suffix = live.phase.suffix()
		if live.label != "" {
			suffix = live.label + " " + suffix
		}
		return base, suffix, true

	default:
		return "", "", false
	}
}
