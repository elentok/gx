package wire

import (
	"time"
)

// Relations a related ticket has to the one on the page.
const (
	RelationBlocker = "blocker"
	RelationParent  = "parent"
	RelationFork    = "fork"
)

// RelatedTicket is one ticket line of the detail page's Related section.
type RelatedTicket struct {
	Relation string `json:"relation"`
	// Address is canonical for a ticket of the same project. A blocker that
	// names no ticket keeps the token as written.
	Address string `json:"address"`
	// Linkable is false for a blocker that names no ticket, which has nothing to link to.
	Linkable bool   `json:"linkable"`
	Title    string `json:"title"`
	// Status is the rendered-status word the epic rows use.
	Status string `json:"status"`
}

// DetailStats is the stat line of a loop ticket. Cost and ContextWindow come
// from the merge stamps and stay empty until the ticket merges. A map ticket
// has no stats.
type DetailStats struct {
	// Elapsed is in seconds; a claimed ticket ticks on the client from ClaimedAt.
	Elapsed       int       `json:"elapsed,omitempty"`
	ClaimedAt     time.Time `json:"claimed_at,omitzero"`
	Cost          float64   `json:"cost,omitempty"`
	HadCodex      bool      `json:"had_codex,omitempty"`
	ContextWindow int       `json:"context_window,omitempty"`
}
