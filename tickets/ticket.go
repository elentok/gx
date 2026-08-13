// Package tickets parses and loads the local markdown issue tracker under
// `.scratch/` (see .ai's issue-tracker-local skill for the on-disk
// conventions), mirroring the role the git package plays for ui/prs.
package tickets

import (
	"strconv"
	"strings"

	"github.com/elentok/gx/tickets/schema"
)

// Ticket is one parsed `<epic>/issues/NN-<slug>.md` file. Number, Title, and
// Path are filled in by the loader from the filename/path; the rest comes
// from parsing the file's frontmatter (see tickets/schema.ParseTicketFromRaw).
type Ticket struct {
	// Number is the numeric portion of the ticket identifier, used for
	// scheduler ordering. Identifier is the full filename prefix, so tickets
	// such as 10a and 10b remain distinct in the UI.
	Number     int
	Identifier string
	Title      string
	Path       string

	Type string
	// BlockedBy holds each "Blocked by:" token as written, e.g. "02" or
	// "04a" (see parseBlockedBy). A bare-number token means the whole
	// number family (Epic.UnresolvedBlockers); a lettered token names one
	// specific sibling.
	BlockedBy []string
	// Parent is the ID of the ticket this one was forked off from
	// (schema.Ticket.Parent) — the only stored direction of the fork edge.
	// A ticket's children are derived by scanning its epic for tickets
	// pointing back at it (Epic.forkChildren).
	Parent *string
	Status string // raw Status: value; "" only for a ticket that failed to load (see ReadErr)
	Body   string // raw markdown after the leading metadata lines, unmodified

	// ActualContextWindow and ElapsedTime are the landing-time metrics
	// ralphloop/report_metrics.go's writeLandedMetrics stamps into a done
	// ticket's frontmatter. Both zero for a ticket that's never run, or one
	// landed before metrics existed (report.go's readSessionStats found
	// nothing to stamp).
	ActualContextWindow int
	ElapsedTime         int
	// Compactions mirrors schema.Ticket.Compactions: how many compaction
	// boundaries the landing iteration's session crossed.
	Compactions int

	// Commitless mirrors schema.Ticket.Commitless: true means a zero-commit
	// iteration finish is intentional for this ticket, not a stalled agent.
	Commitless bool

	// ParkKind mirrors schema.Ticket.ParkKind: which of ralph-loop's
	// needs-answer producers parked this ticket, empty for a ticket parked
	// before that field existed (or never parked).
	ParkKind schema.ParkKind

	// Mutes mirrors schema.Ticket.Mutes: notification event types that have
	// tripped a throttle for this ticket, oldest first.
	Mutes []schema.MuteRecord

	// ReadErr is set when the loader found the file (its name matched
	// "NN-<slug>.md") but couldn't read its contents (I/O error). Non-empty
	// means Type/BlockedBy/Status/Body are all zero-valued - there was no
	// content to parse. Distinct from an unrecognized Status: value, which is
	// a successfully-read file.
	ReadErr string

	// GraphErr is set when the ticket parsed fine on its own but its Parent
	// edge is invalid in the context of the whole epic — dangling, or closing
	// a cycle (see Epic.ValidateParentGraph). The loader drops the edge and
	// records the reason here; like ReadErr it renders as StatusError, but it
	// stays a separate field because the file itself is readable and valid.
	GraphErr string
}

// DisplayNumber returns the filename's complete ticket identifier. Tickets
// constructed outside the loader predate Identifier, so fall back to Number
// for those callers.
func (t Ticket) DisplayNumber() string {
	if t.Identifier != "" {
		return t.Identifier
	}
	return strconv.Itoa(t.Number)
}

// doneStatuses is a single-entry set on purpose. It once accepted a wider
// "done family" (resolved/wontfix/closed/implemented); the contracted status
// enum (schema.Status) has exactly one spelling per state, and the loader
// rejects every other value, so those aliases were both unreachable and a
// second, disagreeing status vocabulary sitting next to the canonical one.
var doneStatuses = map[string]bool{
	"done": true,
}

// IsDone reports whether the ticket's raw Status is done. Used for epic
// open/total counts; note that a done ticket whose fork subtree is unfinished
// still renders as StatusWaitingForChildren (see Epic.RenderedStatus).
func (t Ticket) IsDone() bool {
	return doneStatuses[strings.ToLower(strings.TrimSpace(t.Status))]
}
