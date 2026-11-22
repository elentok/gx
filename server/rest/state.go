// Package rest is the pure core of the Rest engine: Bedtime and Break Rest
// periods. It reads no clock and does no I/O; Step takes the state, the
// settings, the current time and one event, and returns the new state.
package rest

import (
	"time"

	"github.com/elentok/gx/config"
)

// Kind is the kind of a Rest period.
type Kind string

const (
	KindBreak   Kind = "break"
	KindBedtime Kind = "bedtime"
)

// Period is the running Rest period. Treat it as immutable: Step replaces it
// instead of editing it.
type Period struct {
	Kind  Kind      `json:"kind"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	// Message and SubMessage are the Break text chosen at the start, so every
	// browser shows the same words. Bedtime text is rendered from the settings.
	Message    string `json:"message,omitempty"`
	SubMessage string `json:"sub_message,omitempty"`
}

// State is everything the engine remembers between events.
type State struct {
	// WorkCount is the number of active minutes since the last reset.
	WorkCount int
	// LastActive is the last counted minute, truncated to the minute.
	LastActive time.Time
	// Period is the running Rest period, nil when none runs.
	Period *Period
	// QuietUntil is the original end of the Rest period an unlock ended.
	QuietUntil time.Time
	// UnlockedKind is the kind of that unlocked Rest period, empty once
	// QuietUntil has passed.
	UnlockedKind Kind
}

// Quiet is true while the feature is on and either a Rest period runs or
// QuietUntil is in the future. Every notification path reads this one check.
func (s State) Quiet(cfg config.MentalHealthConfig, now time.Time) bool {
	return cfg.Enabled && (s.Period != nil || now.Before(s.QuietUntil))
}

// RecordKind tags the records Step asks the caller to persist.
type RecordKind string

const (
	RecordBreakStart   RecordKind = "break-start"
	RecordBreakDropped RecordKind = "break-dropped"
	RecordUnlock       RecordKind = "unlock"
)

// Record is one entry for the append-only store. Bedtime starts are not
// recorded.
type Record struct {
	Kind RecordKind `json:"kind"`
	Time time.Time  `json:"time"`
	// Started is the start time of the Break a RecordBreakDropped cancels.
	Started time.Time `json:"started,omitzero"`
	// Rest is the kind of Rest period a RecordUnlock ended.
	Rest Kind `json:"rest,omitempty"`
}

// Warning is the one upcoming Rest period the person is warned about. A moved
// Start is the same warning with new text.
type Warning struct {
	Kind  Kind
	Start time.Time
	// BreakMinutes is the Break length, set for a Break warning.
	BreakMinutes int
}

// View is what the server publishes for one state.
type View struct {
	// Now is the time the view was computed for, in the location it was given.
	Now time.Time
	// Counting is true while an active minute would count.
	Counting bool
	// Period is the running period with its text rendered into Message.
	Period  *Period
	Warning *Warning
	quiet   bool
}

// Quiet is State.Quiet as of the view's time.
func (v View) Quiet() bool { return v.quiet }

// EventKind selects what happened.
type EventKind int

const (
	// EventTick is the passage of time with nothing else.
	EventTick EventKind = iota
	// EventActive is a heartbeat from any device.
	EventActive
	// EventUnlock is an unlock attempt carrying the typed sentence.
	EventUnlock
	// EventSettings is a change of the settings passed in Input.Config.
	EventSettings
)

// Event is one input to Step.
type Event struct {
	Kind EventKind
	// Sentence is the typed unlock sentence, for EventUnlock.
	Sentence string
}

// Input is everything Step needs.
type Input struct {
	State  State
	Config config.MentalHealthConfig
	// Now carries the location Bedtime is read in.
	Now   time.Time
	Event Event
	// Rand returns a random int in [0, n). Nil means math/rand/v2.
	Rand func(n int) int
	// BreakMessage and BreakSub, when BreakMessage is set, are the text of a
	// Break that starts, ahead of the settings and the built-in choices.
	BreakMessage, BreakSub string
}

// Result is the outcome of one Step.
type Result struct {
	State   State
	Records []Record
	View    View
	// Refusal is the reason an unlock was refused, empty otherwise. A refused
	// unlock changes nothing but the passage of time.
	Refusal string
}
