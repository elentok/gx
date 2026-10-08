// Package recovery owns the recovery catalog: failure signatures, each mapped
// to who handles it (a rule or an agent), how much authority it has, and which
// remedy verbs it may use. The catalog is Go data and the matcher is pure.
package recovery

import (
	"regexp"
	"slices"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/transcript"
)

type Executor string

const (
	ExecutorRule  Executor = "rule"
	ExecutorAgent Executor = "agent"
)

type Authority string

const (
	AuthorityLow    Authority = "low"
	AuthorityMedium Authority = "medium"
	AuthorityHigh   Authority = "high"
)

// Event is the slice of a run-log event the matcher reads.
type Event struct {
	Type events.Type
	Kind events.Kind
	// Text is the iteration's last assistant text, set on the failure event
	// only when the matcher's caller read the transcript.
	Text string
}

// Entry is one catalogued failure. It matches when the newest event in the
// sequence has Type and Kind and Predicate (if any) accepts the whole sequence.
type Entry struct {
	ID        string                 `json:"id"`
	Type      events.Type            `json:"type"`
	Kind      events.Kind            `json:"kind"`
	Predicate func(seq []Event) bool `json:"-"`
	Executor  Executor               `json:"executor"`
	Authority Authority              `json:"authority"`
	// Verbs are the remedy verbs the entry may apply.
	Verbs   []string `json:"verbs"`
	Enabled bool     `json:"enabled"`
	// Remedy is the Go fix of a rule entry; nil for agent entries.
	Remedy Remedy `json:"-"`
}

// Catalog is the set of entries plus the global kill switch.
type Catalog struct {
	Enabled bool    `json:"enabled"`
	Entries []Entry `json:"entries"`
}

// NotCatalogued records failures deliberately without an entry.
var NotCatalogued = map[string]string{
	"R13": "vanishes under the server's durable queue",
}

// Default is the shipped catalog: kill switch on, one entry per R-ticket as
// they land.
func Default() Catalog {
	entries := []Entry{r1Spin(), r2UnexecutedToolCall()}
	return Catalog{Enabled: true, Entries: append(entries, r3LandRecoverable()...)}
}

var claimsWorkPresentRE = regexp.MustCompile(`(?i)\balready (present|implemented|merged|landed|exists?|done|on the (feature )?branch)\b`)

// r3LandRecoverable is work that is on the feature branch (or a recoverable
// branch) but gx cannot attribute: a done ticket whose commits are missing, or
// a zero-commit finish whose last turn claims the work is already present.
// One catalogued failure with two signatures, so two entries sharing the ID
// (a disable by ID covers both). An agent proves presence with verify and the
// acceptance criteria before it lands a recoverable branch. Launches
// disabled: there is no S0 event data showing it occurs yet.
func r3LandRecoverable() []Entry {
	base := Entry{
		ID: "R3", Executor: ExecutorAgent, Authority: AuthorityMedium,
		Verbs: []string{"verify", "land"},
	}
	unrecoverable := base
	unrecoverable.Type, unrecoverable.Kind = events.NeedsRepair, events.AmbiguousLand
	claimed := base
	claimed.Type, claimed.Kind = events.NeedsAnswer, events.ZeroCommit
	claimed.Predicate = func(seq []Event) bool {
		return claimsWorkPresentRE.MatchString(seq[len(seq)-1].Text)
	}
	return []Entry{unrecoverable, claimed}
}

// r2UnexecutedToolCall is a zero-commit finish whose last assistant turn is only
// a call literal. Telling that from a real answer needs the transcript, so an
// agent investigates; the verbs are the whole grant (one corrective nudge, then
// one finish-wait, both done by relaunching the iteration). Launches disabled:
// there is no S0 event data showing it occurs yet.
func r2UnexecutedToolCall() Entry {
	return Entry{
		ID: "R2", Type: events.NeedsAnswer, Kind: events.ZeroCommit,
		Predicate: func(seq []Event) bool {
			return transcript.LooksLikeUnexecutedToolCall(seq[len(seq)-1].Text)
		},
		Executor: ExecutorAgent, Authority: AuthorityMedium, Verbs: []string{"relaunch"},
	}
}

// r1Spin records a park/re-claim spin. The S0 spinning park already carries
// the count and window, so there is no predicate; the remedy does nothing
// because a nudge would only join the spin and the park already holds it.
// Launches enabled: S0 emits the event it matches.
func r1Spin() Entry {
	return Entry{
		ID: "R1", Type: events.NeedsRepair, Kind: events.Spinning,
		Executor: ExecutorRule, Authority: AuthorityLow, Enabled: true,
		Remedy: func(Failure, Verbs) error { return nil },
	}
}

// WithConfig applies the user's kill switch and per-entry disables.
func (c Catalog) WithConfig(enabled bool, disabled []string) Catalog {
	out := Catalog{Enabled: c.Enabled && enabled, Entries: slices.Clone(c.Entries)}
	for i := range out.Entries {
		if slices.Contains(disabled, out.Entries[i].ID) {
			out.Entries[i].Enabled = false
		}
	}
	return out
}

// Match returns the first enabled entry matching seq, or false.
func (c Catalog) Match(seq []Event) (Entry, bool) {
	if !c.Enabled || len(seq) == 0 {
		return Entry{}, false
	}
	last := seq[len(seq)-1]
	for _, e := range c.Entries {
		if !e.Enabled || e.Type != last.Type || e.Kind != last.Kind {
			continue
		}
		if e.Predicate != nil && !e.Predicate(seq) {
			continue
		}
		return e, true
	}
	return Entry{}, false
}
