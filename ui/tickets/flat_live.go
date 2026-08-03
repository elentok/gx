package tickets

import (
	"fmt"
	"image/color"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
	"github.com/elentok/gx/ui"
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
	// startedAt is the iteration's session transcript's first-line
	// timestamp (resolved from IterationStarted/TicketReattached's
	// Cwd+SessionID), so elapsed time keeps climbing across a
	// pause/resume instead of resetting. Zero if the transcript couldn't
	// be resolved yet.
	startedAt time.Time
	// tokens is the last LiveEventContextOccupancy reading for this
	// ticket — frozen (not zeroed) while paused, since the underlying
	// session's context hasn't changed.
	tokens int
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
	applyLiveEvent(m.live, m.labelIdentifier, m.transcript, ev)
}

// applyLiveEvent is the shared fold FlatModel and Model (ui/tickets' main
// app tab, ticket 02) both use to turn one orchestrator LiveEvent into
// live/labelIdentifier map updates — Model has no transcript tail, so callers
// that don't track one pass a nil map (checked below; a nil map read is fine,
// only the write needs guarding).
func applyLiveEvent(live map[string]liveTicketState, labelIdentifier map[string]string, transcript map[string][]string, ev ralphloop.LiveEvent) {
	switch ev.Kind {
	case ralphloop.LiveEventIterationStarted, ralphloop.LiveEventTicketReattached:
		labelIdentifier[ev.Label] = ev.Identifier
		live[ev.Identifier] = liveTicketState{running: true, label: ev.Label, startedAt: resolveStartedAt(ev.Cwd, ev.SessionID)}

	case ralphloop.LiveEventIterationPaused:
		if identifier, ok := labelIdentifier[ev.Label]; ok {
			prev := live[identifier]
			live[identifier] = liveTicketState{
				paused: true, label: ev.Label, pauseKind: ev.PauseKind, reason: ev.Reason,
				startedAt: prev.startedAt, tokens: prev.tokens,
			}
		}

	case ralphloop.LiveEventIterationResumed:
		if identifier, ok := labelIdentifier[ev.Label]; ok {
			prev := live[identifier]
			live[identifier] = liveTicketState{running: true, label: ev.Label, startedAt: prev.startedAt, tokens: prev.tokens}
		}

	case ralphloop.LiveEventIterationFinished, ralphloop.LiveEventTicketReverted,
		ralphloop.LiveEventTicketCleanupFinished, ralphloop.LiveEventTicketRecovered:
		delete(live, ev.Identifier)
		if transcript != nil {
			delete(transcript, ev.Identifier)
		}

	case ralphloop.LiveEventTranscriptLine:
		if transcript == nil {
			return
		}
		if identifier, ok := labelIdentifier[ev.Label]; ok {
			lines := append(transcript[identifier], ev.Line)
			if len(lines) > flatTranscriptMaxLines {
				lines = lines[len(lines)-flatTranscriptMaxLines:]
			}
			transcript[identifier] = lines
		}

	case ralphloop.LiveEventTicketStillNeedsAttention:
		live[ev.Identifier] = liveTicketState{
			paused: true, pauseKind: ralphloop.PauseNeedsAttention, reason: "no live iteration to reattach to",
		}

	case ralphloop.LiveEventTicketRecovering:
		live[ev.Identifier] = liveTicketState{running: true}

	case ralphloop.LiveEventTicketUnrecoverable:
		live[ev.Identifier] = liveTicketState{
			paused: true, pauseKind: ralphloop.PauseNeedsAttention, reason: "commits missing from epic; needs operator review",
		}

	case ralphloop.LiveEventCherryPickStarted:
		if ls, ok := live[ev.Identifier]; ok {
			ls.phase = livePhaseCherryPicking
			live[ev.Identifier] = ls
		}

	case ralphloop.LiveEventConflictResolutionStarted:
		if ls, ok := live[ev.Identifier]; ok {
			ls.phase = livePhaseResolvingConflicts
			live[ev.Identifier] = ls
		}

	case ralphloop.LiveEventSmartZoneCompactStarted:
		if ls, ok := live[ev.Identifier]; ok {
			ls.phase = livePhaseCompacting
			live[ev.Identifier] = ls
		}

	case ralphloop.LiveEventSmartZoneFinishingUp:
		if ls, ok := live[ev.Identifier]; ok {
			ls.phase = livePhaseFinishingUp
			live[ev.Identifier] = ls
		}

	case ralphloop.LiveEventSmartZoneRecovered:
		if ls, ok := live[ev.Identifier]; ok {
			ls.phase = livePhaseImplementing
			live[ev.Identifier] = ls
		}

	case ralphloop.LiveEventContextOccupancy:
		if ls, ok := live[ev.Identifier]; ok {
			ls.tokens = ev.Tokens
			live[ev.Identifier] = ls
		}
	}
}

// resolveStartedAt resolves cwd/sessionID (carried on IterationStarted/
// TicketReattached) to the session transcript's first-line timestamp — the
// true iteration start, which survives a TicketReattached (UI restart
// mid-iteration) correctly instead of resetting to zero the way a plain
// time.Now()-at-start capture would. Returns the zero time.Time if the
// transcript can't be resolved yet (e.g. not written out on disk).
func resolveStartedAt(cwd, sessionID string) time.Time {
	path, err := transcript.Path(cwd, sessionID)
	if err != nil {
		return time.Time{}
	}
	startedAt, ok, err := transcript.FirstLineTimestamp(path)
	if err != nil || !ok {
		return time.Time{}
	}
	return startedAt
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
// keeps this function total). base is the bare icon/spinner+title; suffix is
// the phase/label text (running) or pause reason (paused/needs-attention),
// returned separately rather than composed onto one line so each caller can
// place it where it needs to: Model's sidebar (appendBlockedBySuffix, one
// line, unchanged) vs. FlatModel's two-line row, which also mixes in the
// run's live metrics (flat_view.go's renderFlatTicketRow) — icons/sp are the
// two bits of per-model UI state the rendering needs.
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
		spinnerView := lipgloss.NewStyle().Foreground(live.phase.color()).Render(sp.View())
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
