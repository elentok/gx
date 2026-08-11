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

// TicketID is a validated ticket identifier, e.g. "04", "06b", or "06b1":
// two or more digits, an optional single trailing lowercase letter, and —
// only past that letter — optional trailing digits for one extra nesting
// level (a numeric child of a lettered split, see tickets.NextTicketID).
// Real IDs are zero-padded (hence the 2-digit minimum, stricter than the
// spec's literal \d+[a-z]?\d* — a bare "4" is not a real ID seen in
// practice). "06ab" (two letters) and "06b1a" (a letter after a digit
// suffix) don't match: nesting stops one level past a lettered parent.
type TicketID string

var ticketIDRe = regexp.MustCompile(`^\d{2,}[a-z]?\d*$`)

// Valid reports whether id matches the canonical ticket-identifier format.
func (id TicketID) Valid() bool {
	return ticketIDRe.MatchString(string(id))
}

// Status is the ticket lifecycle enum. Unlike the legacy bold-line format,
// there is deliberately no "error" member: an invalid raw string is a
// Validate failure, not a status value. The empty string is not a member
// either — status is required, which is what keeps a hand-authored file with
// no status: and no body from reading as open and becoming schedulable.
type Status string

const (
	// StatusDraft is written down but deliberately not offered to anyone: it
	// never enters an epic's frontier and never renders as open (see
	// tickets.StatusDraft).
	StatusDraft       Status = "draft"
	StatusOpen        Status = "open"
	StatusClaimed     Status = "claimed"
	StatusNeedsAnswer Status = "needs-answer"
	StatusNeedsRepair Status = "needs-repair"
	StatusDone        Status = "done"
)

var validStatuses = map[Status]bool{
	StatusDraft:       true,
	StatusOpen:        true,
	StatusClaimed:     true,
	StatusNeedsAnswer: true,
	StatusNeedsRepair: true,
	StatusDone:        true,
}

// retiredStatusReplacements names every status spelling this clean cut
// retired, mapped to the replacement Validate should point a caller at. It
// exists so a hand-authored file still carrying the old spelling is rejected
// with an actionable error instead of the generic "invalid status" every
// other never-valid value gets — the loader must never silently read
// "needs-info"/"needs-attention" as their new equivalents.
var retiredStatusReplacements = map[Status]Status{
	"needs-info":      StatusNeedsAnswer,
	"needs-attention": StatusNeedsRepair,
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
	// Parent records the "this ticket was produced from another" relationship:
	// a mid-flight budget split, or a fix ticket a code-review ticket opened.
	// It is the only direction the edge is stored in — the reverse (a ticket's
	// children) is derived by scanning an epic for tickets pointing at it, so
	// there is no second copy for a fork to forget to update.
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
	// for this ticket (e.g. the agent decided no code change was warranted,
	// or a mid-flight split closed it with no work of its own) rather than a
	// stalled/crashed agent. ralph-loop's finishIteration checks it before
	// marking a zero-commit ticket needs-answer, and startup reconciliation's
	// classifyDoneTicket skips its landed-commit verification for a done
	// ticket with this set. Prefer IsCommitless over reading this field
	// directly — research/grilling/code-review tickets are commitless by
	// type, without needing the flag set explicitly.
	Commitless bool
}

// IsCommitless reports whether t is exempt from landed-commit verification:
// either explicitly flagged Commitless, or a type that never produces
// commits in the first place. TypeResearch and TypeGrilling tickets record
// their deliverable in the ticket body itself; TypeCodeReview reviews the
// epic rather than landing code of its own — none of the three lands a
// commit on the feature branch even when finished correctly, so requiring
// one would misclassify every one of them as a stalled/crashed agent.
// TypePrototype is deliberately excluded: a prototype can legitimately land
// a real spike/scaffold commit as its actual output, so it stays on the
// crash-recovery path like TypeTask unless explicitly flagged.
func (t Ticket) IsCommitless() bool {
	return t.Commitless || t.Type == TypeResearch || t.Type == TypeGrilling || t.Type == TypeCodeReview
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
		if replacement, retired := retiredStatusReplacements[t.Status]; retired {
			errs = append(errs, fmt.Errorf("status: %q was renamed to %q, run `gx tickets migrate`", t.Status, replacement))
		} else {
			errs = append(errs, fmt.Errorf("status: invalid status %q", t.Status))
		}
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
