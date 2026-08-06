// Package schema defines the typed ticket-frontmatter data model —
// TicketID, Status, TicketType, the Ticket struct, and Validate — per
// .scratch/ticket-frontmatter/spec.md. It is deliberately its own package
// rather than living in tickets/ticket.go: that package already exports a
// Ticket struct and ParseTicket func for today's legacy bold-line format,
// and a same-named redeclaration in the same package won't compile. A later
// ticket in the ticket-frontmatter epic (which adds frontmatter parsing and
// retires the legacy format) reconciles the two into one.
package schema

import (
	"errors"
	"fmt"
	"regexp"
)

// TicketID is a validated ticket identifier, e.g. "04" or "06b": two or more
// digits with an optional single trailing lowercase letter. Real IDs are
// zero-padded (hence the 2-digit minimum, stricter than the spec's literal
// \d+[a-z]? — a bare "4" is not a real ID seen in practice).
type TicketID string

var ticketIDRe = regexp.MustCompile(`^\d{2,}[a-z]?$`)

// Valid reports whether id matches the canonical ticket-identifier format.
func (id TicketID) Valid() bool {
	return ticketIDRe.MatchString(string(id))
}

// Status is the ticket lifecycle enum. Unlike the legacy bold-line format,
// there is deliberately no "error" member: an invalid raw string is a
// Validate failure, not a status value.
type Status string

const (
	StatusOpen           Status = "open"
	StatusNeedsTriage    Status = "needs-triage"
	StatusReadyForAgent  Status = "ready-for-agent"
	StatusReadyForHuman  Status = "ready-for-human"
	StatusClaimed        Status = "claimed"
	StatusNeedsInfo      Status = "needs-info"
	StatusNeedsAttention Status = "needs-attention"
	StatusDone           Status = "done"
	StatusSuperseded     Status = "superseded"
)

var validStatuses = map[Status]bool{
	StatusOpen:           true,
	StatusNeedsTriage:    true,
	StatusReadyForAgent:  true,
	StatusReadyForHuman:  true,
	StatusClaimed:        true,
	StatusNeedsInfo:      true,
	StatusNeedsAttention: true,
	StatusDone:           true,
	StatusSuperseded:     true,
}

// Valid reports whether s is one of the canonical status values.
func (s Status) Valid() bool {
	return validStatuses[s]
}

// TicketType is the ticket-kind enum (the frontmatter `type` field).
type TicketType string

const (
	TypeResearch   TicketType = "research"
	TypeGrilling   TicketType = "grilling"
	TypePrototype  TicketType = "prototype"
	TypeTask       TicketType = "task"
	TypeCodeReview TicketType = "code-review"
)

var validTypes = map[TicketType]bool{
	TypeResearch:   true,
	TypeGrilling:   true,
	TypePrototype:  true,
	TypeTask:       true,
	TypeCodeReview: true,
}

// Valid reports whether t is one of the canonical type values.
func (t TicketType) Valid() bool {
	return validTypes[t]
}

// Ticket is the in-memory, typed frontmatter of one ticket file's YAML
// header (see .scratch/ticket-frontmatter/spec.md's Schema section). No
// file I/O or YAML (de)serialization here — that's a later ticket in this
// epic; this type is built and inspected directly in tests.
type Ticket struct {
	ID        TicketID
	Status    Status
	BlockedBy []TicketID
	// Children/Parent record the "this ticket produced other tickets"
	// relationship: a mid-flight budget split, or a code-review ticket
	// recording which fix tickets it opened. On disk this may be spelled
	// either children/parent (current) or the legacy split/split_from
	// (read-only compatibility — see ticketYAML.toTicket); every in-memory
	// consumer only ever sees these two fields.
	Children              []TicketID
	Parent                *TicketID
	Type                  TicketType
	ExpectedContextWindow int
	ActualContextWindow   int
	ElapsedTime           int
	Compactions           int
	// SessionIDs accumulates the native session id of every claude/codex
	// instance ever launched or reattached for this ticket, oldest first —
	// distinct from the dropped single-value Session field (see
	// MarkDoneWithMetadata's doc comment) in that it appends across
	// reattaches/resumes rather than being overwritten by the latest one.
	SessionIDs []string
	// Commitless declares that a zero-commit iteration finish is intentional
	// for this ticket (e.g. the agent decided no code change was warranted)
	// rather than a stalled/crashed agent. ralph-loop's finishIteration
	// checks it before marking a zero-commit ticket needs-info, and startup
	// reconciliation's classifyDoneTicket skips its landed-commit
	// verification for a done ticket with this set, the same way it already
	// does for StatusSuperseded. Prefer IsCommitless over reading this field
	// directly — grilling/prototype tickets are commitless by type, without
	// needing the flag set explicitly.
	Commitless bool
}

// IsCommitless reports whether t is exempt from landed-commit verification:
// either explicitly flagged Commitless, or a type that never produces
// commits in the first place. TypeGrilling tickets record a decision in
// their own body; TypePrototype tickets explore in a throwaway iteration
// worktree — neither lands a commit on the feature branch even when
// finished correctly, so requiring one would misclassify every one of them
// as a stalled/crashed agent.
func (t Ticket) IsCommitless() bool {
	return t.Commitless || t.Type == TypeGrilling || t.Type == TypePrototype
}

// Validate checks a populated Ticket for well-formedness: a valid id, a
// valid status, a valid type, non-negative numeric fields, and no
// blocked_by/parent entry equal to the ticket's own id. Every failing field
// is reported at once via errors.Join, rather than stopping at the first.
func Validate(t Ticket) error {
	var errs []error

	if !t.ID.Valid() {
		errs = append(errs, fmt.Errorf("id: invalid ticket ID %q", t.ID))
	}
	if !t.Status.Valid() {
		errs = append(errs, fmt.Errorf("status: invalid status %q", t.Status))
	}
	if !t.Type.Valid() {
		errs = append(errs, fmt.Errorf("type: invalid type %q", t.Type))
	}
	if t.ExpectedContextWindow < 0 {
		errs = append(errs, fmt.Errorf("expected_context_window: must be non-negative, got %d", t.ExpectedContextWindow))
	}
	if t.ActualContextWindow < 0 {
		errs = append(errs, fmt.Errorf("actual_context_window: must be non-negative, got %d", t.ActualContextWindow))
	}
	if t.ElapsedTime < 0 {
		errs = append(errs, fmt.Errorf("elapsed_time: must be non-negative, got %d", t.ElapsedTime))
	}
	if t.Compactions < 0 {
		errs = append(errs, fmt.Errorf("compactions: must be non-negative, got %d", t.Compactions))
	}
	for _, b := range t.BlockedBy {
		if b == t.ID {
			errs = append(errs, fmt.Errorf("blocked_by: self-reference %q", b))
			break
		}
	}
	if t.Parent != nil && *t.Parent == t.ID {
		errs = append(errs, fmt.Errorf("parent: self-reference %q", *t.Parent))
	}

	return errors.Join(errs...)
}
