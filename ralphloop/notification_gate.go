package ralphloop

import (
	"strings"
	"time"

	"github.com/elentok/gx/tickets/schema"
)

// GateDecision is NotificationGate's verdict for one (transport, eventType,
// source) call.
type GateDecision int

const (
	Allowed GateDecision = iota
	PerSourceMuted
	GloballyMuted
)

// ParkFunc is the optional callback NotificationGate calls when it trips a per-source
// mute, identifying the source and giving a human-readable reason. A bare
// `gx notify` invocation with no loop context passes nil — the mute still
// gets written, the park just gets skipped.
type ParkFunc func(source, reason string) error

// GateResult is NotificationGate's return value: the decision plus whether this call is
// the edge-triggered one that should also send a "muting this"/"globally
// muted" notification (never fired again for the same already-tripped
// state).
type GateResult struct {
	Decision      GateDecision
	EdgeTriggered bool
}

const (
	// perSourceThreshold/perSourceWindow: 5 identical (event type, source)
	// events within a trailing minute trips a per-source mute.
	perSourceThreshold = 5
	perSourceWindow    = 60 * time.Second

	// globalThreshold/globalWindow: ~20 sends/min (send series, all sources
	// combined) trips a transport-wide mute.
	globalThreshold = 20
	globalWindow    = 60 * time.Second

	// globalAttributionShare: a source responsible for at least this share of
	// the event series' trailing window also gets individually muted on a
	// global trip.
	globalAttributionShare = 0.25

	// seriesRetention bounds how long Events/Sends entries are kept on disk —
	// comfortably past both windows above, so nothing needed for a decision
	// is ever trimmed away first.
	seriesRetention = 5 * time.Minute
)

// NotificationGate is the pure decision function every notification send path will
// eventually call through. It always records this call in the transport's
// trailing event series; recordSend additionally records one send-series
// entry (a caller's responsibility per send, whether that's one immediate
// send or one batched flush covering many events).
//
// source is either a ticket's file path (so a per-source mute can be written
// onto it) or a ticket-less sentinel ("cli", "epic:<name>") — a ticket-less
// source is counted toward both series but can never itself be
// per-source-muted.
func NotificationGate(transport, eventType, source string, now time.Time, recordSend bool, parkTicket ParkFunc) (GateResult, error) {
	path, err := notificationStateFilePathFn()
	if err != nil {
		return GateResult{}, err
	}
	return notificationGateAt(path, transport, eventType, source, now, recordSend, parkTicket)
}

func notificationGateAt(path, transport, eventType, source string, now time.Time, recordSend bool, parkTicket ParkFunc) (GateResult, error) {
	var result GateResult
	var opErr error

	err := updateNotificationStateAt(path, func(state *NotificationState) {
		result, opErr = applyGate(state, transport, eventType, source, now, recordSend, parkTicket)
	})
	if err != nil {
		return GateResult{}, err
	}
	return result, opErr
}

func applyGate(state *NotificationState, transport, eventType, source string, now time.Time, recordSend bool, parkTicket ParkFunc) (GateResult, error) {
	ts := state.Transports[transport]
	defer func() { state.Transports[transport] = ts }()

	if ts.Muted {
		return GateResult{Decision: GloballyMuted}, nil
	}

	ts.Events = append(trimEvents(ts.Events, now), NotificationEvent{EventType: eventType, Source: source, Time: now})
	if recordSend {
		ts.Sends = append(trimSends(ts.Sends, now), NotificationSend{Time: now})
	}

	result := GateResult{Decision: Allowed}
	ticketless := isTicketlessSource(source)

	alreadyMuted := false
	if !ticketless {
		var err error
		alreadyMuted, err = ticketAlreadyMuted(source, eventType)
		if err != nil {
			return GateResult{}, err
		}
	}

	switch {
	case alreadyMuted:
		result.Decision = PerSourceMuted
	case !ticketless && countMatchingInWindow(ts.Events, now, perSourceWindow, eventType, source) >= perSourceThreshold:
		edge, err := muteSource(source, eventType, now, parkTicket, "storm mute: per-source threshold crossed")
		if err != nil {
			return GateResult{}, err
		}
		result.Decision = PerSourceMuted
		result.EdgeTriggered = edge
	}

	if recordSend && countSendsInWindow(ts.Sends, now, globalWindow) >= globalThreshold {
		breakdown := eventBreakdown(ts.Events, now, globalWindow)
		total := 0
		for _, sc := range breakdown {
			total += sc.Count
		}
		for _, sc := range breakdown {
			if total == 0 || float64(sc.Count)/float64(total) < globalAttributionShare {
				continue
			}
			if _, err := muteSource(sc.Source, eventType, now, parkTicket, "storm mute: global-trip attribution"); err != nil {
				return GateResult{}, err
			}
		}

		ts.Muted = true
		ts.TrippedAt = now
		ts.Reason = "auto-trip"
		ts.Trips = append(ts.Trips, TransportTrip{TrippedAt: now, Reason: "auto-trip", Sources: breakdown})

		result.Decision = GloballyMuted
		result.EdgeTriggered = true
	}

	return result, nil
}

// isTicketlessSource reports whether source is one of the sentinel values
// used when there's no ticket to write a mute onto ("cli", "epic:<name>").
func isTicketlessSource(source string) bool {
	return source == "cli" || strings.HasPrefix(source, "epic:")
}

// ticketAlreadyMuted reports whether source's ticket already carries a
// MuteRecord for eventType — the sticky, manual-clear-only check that keeps
// a mute in force even once the burst that tripped it has aged out of the
// trailing window.
func ticketAlreadyMuted(source, eventType string) (bool, error) {
	t, err := schema.ParseTicket(source)
	if err != nil {
		return false, err
	}
	for _, m := range t.Mutes {
		if m.EventType == eventType {
			return true, nil
		}
	}
	return false, nil
}

// muteSource writes a per-source mute onto source's ticket and, if supplied,
// calls parkTicket. It's idempotent (re-reads the ticket's current mute
// state) and a no-op for a ticket-less source, so both the per-source
// threshold trip and the global-attribution trip can call it without
// double-writing or double-parking the same source.
func muteSource(source, eventType string, now time.Time, parkTicket ParkFunc, reason string) (bool, error) {
	if isTicketlessSource(source) {
		return false, nil
	}

	already, err := ticketAlreadyMuted(source, eventType)
	if err != nil {
		return false, err
	}
	if already {
		return false, nil
	}

	if err := schema.UpdateTicket(source, func(t *schema.Ticket) {
		t.Mutes = append(t.Mutes, schema.MuteRecord{EventType: eventType, TrippedAt: now})
	}); err != nil {
		return false, err
	}

	if parkTicket != nil {
		if err := parkTicket(source, reason); err != nil {
			return false, err
		}
	}
	return true, nil
}

func trimEvents(events []NotificationEvent, now time.Time) []NotificationEvent {
	cutoff := now.Add(-seriesRetention)
	out := make([]NotificationEvent, 0, len(events))
	for _, e := range events {
		if !e.Time.Before(cutoff) {
			out = append(out, e)
		}
	}
	return out
}

func trimSends(sends []NotificationSend, now time.Time) []NotificationSend {
	cutoff := now.Add(-seriesRetention)
	out := make([]NotificationSend, 0, len(sends))
	for _, s := range sends {
		if !s.Time.Before(cutoff) {
			out = append(out, s)
		}
	}
	return out
}

func countMatchingInWindow(events []NotificationEvent, until time.Time, window time.Duration, eventType, source string) int {
	cutoff := until.Add(-window)
	count := 0
	for _, e := range events {
		if e.Time.Before(cutoff) || e.Time.After(until) {
			continue
		}
		if e.EventType == eventType && e.Source == source {
			count++
		}
	}
	return count
}

func countSendsInWindow(sends []NotificationSend, until time.Time, window time.Duration) int {
	cutoff := until.Add(-window)
	count := 0
	for _, s := range sends {
		if !s.Time.Before(cutoff) && !s.Time.After(until) {
			count++
		}
	}
	return count
}

// eventBreakdown returns every source active in the trailing window (event
// series), muted or not, in first-seen order — the shape TransportTrip.
// Sources and the global-attribution share check both need.
func eventBreakdown(events []NotificationEvent, until time.Time, window time.Duration) []SourceCount {
	cutoff := until.Add(-window)
	counts := map[string]int{}
	var order []string
	for _, e := range events {
		if e.Time.Before(cutoff) || e.Time.After(until) {
			continue
		}
		if _, ok := counts[e.Source]; !ok {
			order = append(order, e.Source)
		}
		counts[e.Source]++
	}
	out := make([]SourceCount, 0, len(order))
	for _, s := range order {
		out = append(out, SourceCount{Source: s, Count: counts[s]})
	}
	return out
}
