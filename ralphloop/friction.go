package ralphloop

import (
	"slices"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// unknownFriction labels a pause with no recorded kind and a pause end with no
// recorded source.
const unknownFriction = "unknown"

// frictionRunEvents are the run events a Friction summary counts. Anything else
// (nudges, deadlocks, smart-zone pauses, late-finish adoptions, relaunches,
// successful auto-recovery) is left out. The value is the series it counts under.
var frictionRunEvents = map[events.Type]events.Type{
	events.ConflictHit:       events.ConflictHit,
	events.TicketStartedOver: events.TicketStartedOver,
	events.ClaimReopened:     events.TicketStartedOver,
	events.ManualMerge:       events.ManualMerge,
	events.RecoveryEscalated: events.RecoveryEscalated,
	events.PausedRateLimit:   events.PausedRateLimit,
	events.LaunchFailed:      events.LaunchFailed,
}

// Friction builds the Friction summary of one ticket. runEvents is the epic's
// run log as ReadEvents returns it (legacy names already mapped); log is the
// ticket's own log.
//
// A ticket with Pause rounds counts one pause per round. A ticket without
// (old data) counts one pause per spell in the run log: a spell starts at the
// first paused event after the ticket last started an iteration, so repeated
// paused lines count once, and it ends at the next start or resume.
func Friction(ticket string, runEvents []Event, log []tickets.LogEvent) schema.FrictionSummary {
	var f schema.FrictionSummary
	var mine []Event
	for _, ev := range runEvents {
		if ev.Ticket != ticket {
			continue
		}
		mine = append(mine, ev)
		if series, ok := frictionRunEvents[events.Type(ev.Type)]; ok {
			bump(&f.RunEvents, string(series))
		}
	}

	if rounds := tickets.AllRounds(log); len(rounds) > 0 {
		for _, r := range rounds {
			frictionRound(&f, r)
		}
		return f
	}
	frictionSpells(&f, mine)
	return f
}

func frictionRound(f *schema.FrictionSummary, r tickets.PauseRound) {
	bump(&f.PausesByReason, orUnknown(string(events.CurrentKind(events.Kind(r.Question.PauseKind)))))
	if r.End == nil {
		return
	}
	source := orUnknown(r.End.Source)
	bump(&f.PauseEndings, source)
	secs := int(r.End.TS.Sub(r.Question.TS).Seconds())
	f.TotalPausedSecs += secs
	if slices.Contains([]string{tickets.SourceWeb, tickets.SourceTUI, tickets.SourceCLI}, source) {
		f.PersonPauseSecs = append(f.PersonPauseSecs, secs)
	}
}

func frictionSpells(f *schema.FrictionSummary, evs []Event) {
	var start *Event
	for i, ev := range evs {
		switch events.Type(ev.Type) {
		case events.Paused:
			if start != nil {
				continue
			}
			start = &evs[i]
			bump(&f.PausesByReason, orUnknown(ev.Kind))
			bump(&f.PauseEndings, unknownFriction)
		case events.IterationStarted, events.Resumed:
			if start != nil {
				f.TotalPausedSecs += int(ev.Time.Sub(start.Time).Seconds())
			}
			start = nil
		}
	}
}

func bump(m *map[string]int, key string) {
	if *m == nil {
		*m = map[string]int{}
	}
	(*m)[key]++
}

func orUnknown(s string) string {
	if s == "" {
		return unknownFriction
	}
	return s
}
