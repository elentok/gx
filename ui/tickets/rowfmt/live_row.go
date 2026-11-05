package rowfmt

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

var (
	// StatusPausedRepairStyle colors a repair-paused ticket's icon.
	StatusPausedRepairStyle = lipgloss.NewStyle().Foreground(ui.ColorRed)

	// StatusPausedStyle renders a live orchestrator pause (rate-limit/
	// smart-zone — see ralph-loop's FlatModel), distinct from every disk-only
	// status color.
	StatusPausedStyle = lipgloss.NewStyle().Foreground(ui.ColorMauve)

	BlockedBySuffixStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtleLight).Italic(true)
)

// LiveTicketState is a ticket's in-memory-only orchestrator state — which
// iteration is running/paused and why — layered on top of its on-disk
// Status: line (ticket 01 keeps ticket status itself on disk; only this
// comes from the server's ticket snapshot). A ticket absent from Model.live has
// had no snapshot touch it yet, so its row falls back to ticket 03's
// disk-only rendering unchanged.
type LiveTicketState struct {
	Running   bool
	Paused    bool
	Label     string // iteration/tab label, e.g. "iter-04a"
	PauseKind runlog.PauseKind
	Reason    string
	Phase     LivePhase
	// WaitingOn is the conflict-resolution child a merging ticket waits on;
	// when set it replaces the phase text.
	WaitingOn string
	// StartedAt is this ticket's own start time, stamped when its iteration
	// begins, so elapsed time keeps climbing across
	// a pause/resume instead of resetting, and doesn't conflate two tickets in
	// the same run. Zero if the ticket hasn't started.
	StartedAt time.Time
	// Tokens is the last context-occupancy reading for this ticket —
	// frozen (not zeroed) while paused, since the underlying session's
	// context hasn't changed.
	Tokens int
}

// LiveElapsedSeconds computes a running/paused ticket's elapsed time from
// live.StartedAt (that ticket's own start time), so it keeps climbing across
// renders without a UI-side stopwatch. Zero if StartedAt hasn't been set.
func LiveElapsedSeconds(live LiveTicketState) int {
	if live.StartedAt.IsZero() {
		return 0
	}
	return int(time.Since(live.StartedAt).Seconds())
}

// LivePhase distinguishes what a running iteration is doing right now, so
// RenderLiveTicketRow can show a more specific row-suffix than a bare
// spinner+label. Zero value (LivePhaseImplementing) covers the whole span
// from start until the agent's commits are cherry-picked — the common case,
// so it needs no dedicated event to enter.
type LivePhase int

const (
	LivePhaseImplementing LivePhase = iota
	LivePhaseCherryPicking
	LivePhaseResolvingConflicts
	LivePhaseCompacting
	LivePhaseFinishingUp
	LivePhaseMerging
)

// suffix returns p's row-suffix text, e.g. "(implementing...)".
func (p LivePhase) suffix() string {
	switch p {
	case LivePhaseCherryPicking:
		return "(cherry-picking...)"
	case LivePhaseResolvingConflicts:
		return "(resolving conflicts...)"
	case LivePhaseCompacting:
		return "(compacting...)"
	case LivePhaseFinishingUp:
		return "(telling the agent to finish up...)"
	case LivePhaseMerging:
		return "(merging...)"
	default:
		return "(implementing...)"
	}
}

// color returns p's accent color, so a running row's spinner distinguishes
// what the iteration is doing right now at a glance rather than relying
// solely on the text suffix (which reads as the same dim italic for every
// phase).
func (p LivePhase) color() color.Color {
	switch p {
	case LivePhaseCherryPicking:
		return ui.ColorTeal
	case LivePhaseResolvingConflicts:
		return ui.ColorRed
	case LivePhaseCompacting:
		return ui.ColorYellow
	case LivePhaseFinishingUp:
		return ui.ColorOrange
	case LivePhaseMerging:
		return ui.ColorMauve
	default:
		return ui.ColorBlue
	}
}

// RenderLiveTicketRow renders t's row from its live orchestrator state
// instead of ticket 03's disk-only rendering, when live is running or
// paused. ok is false if live has neither (the zero value, which shouldn't
// reach here since callers only look live up on a present map entry, but
// keeps this function total). base is the bare icon/spinner+title; suffix is
// the phase/label text (running) or pause reason (paused/paused (repair)),
// returned separately so the caller can place it where it needs to. prefix is
// the caller's own leading-column indent (e.g. the Tickets tab's checkbox
// column vs. the Queue tab's bare indent), so a live row's indent matches its
// tab's non-live rows rather than a value hardcoded here.
func RenderLiveTicketRow(icons ui.IconSet, sp spinner.Model, t tickets.Ticket, live LiveTicketState, prefix string) (base, suffix string, ok bool) {
	title := RowTitle(icons, t)

	switch {
	case live.Paused && live.PauseKind == runlog.PauseRepair:
		base = prefix + StatusPausedRepairStyle.Render(icons.TicketPausedRepair) + " " + title
		return base, live.Reason, true

	case live.Paused:
		base = prefix + StatusPausedStyle.Render(icons.TicketPaused) + " " + title
		return base, live.Reason, true

	case live.Running:
		// TrimRight guards against any spinner whose frames carry trailing
		// whitespace, keeping the icon column single-width and aligned with
		// the other status glyphs.
		spinnerView := lipgloss.NewStyle().Foreground(live.Phase.color()).Render(strings.TrimRight(sp.View(), " "))
		base = prefix + spinnerView + " " + title
		suffix = live.Phase.suffix()
		if live.WaitingOn != "" {
			suffix = fmt.Sprintf("(waiting on %s...)", live.WaitingOn)
		}
		if live.Label != "" {
			suffix = live.Label + " " + suffix
		}
		return base, suffix, true

	default:
		return "", "", false
	}
}
