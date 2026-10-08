// Package recovery owns the recovery catalog: failure signatures, each mapped
// to who handles it (a rule or an agent), how much authority it has, and which
// remedy verbs it may use. The catalog is Go data and the matcher is pure.
package recovery

import (
	"slices"

	"github.com/elentok/gx/events"
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
	return Catalog{Enabled: true}
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
