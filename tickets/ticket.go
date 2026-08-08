// Package tickets parses and loads the local markdown issue tracker under
// `.scratch/` (see .ai's issue-tracker-local skill for the on-disk
// conventions), mirroring the role the git package plays for ui/prs.
package tickets

import (
	"strconv"
	"strings"
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
	// Children holds the IDs of the tickets this one was forked into
	// (mid-flight fork, see .ai's to-tickets skill); Parent is the reverse
	// edge, the ID of the ticket this one was forked off from
	// (schema.Ticket.Parent). At most one of a ticket's Children/Parent is
	// meaningfully populated in practice, but both are carried since
	// schema.Ticket allows either.
	Children []string
	Parent   *string
	Status   string // raw Status: value; "" means missing (valid open/unclaimed default)
	Body     string // raw markdown after the leading metadata lines, unmodified

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

	// ReadErr is set when the loader found the file (its name matched
	// "NN-<slug>.md") but couldn't read its contents (I/O error). Non-empty
	// means Type/BlockedBy/Status/Body are all zero-valued - there was no
	// content to parse. Distinct from an unrecognized Status: value, which is
	// a successfully-read file.
	ReadErr string
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

var doneStatuses = map[string]bool{
	"done":        true,
	"resolved":    true,
	"wontfix":     true,
	"closed":      true,
	"implemented": true,
}

// IsDone reports whether the ticket's raw Status collapses into the "done"
// family. Used for epic open/total counts; the full five-state rendered
// status (open/claimed/blocked/needs-info/done) is a later concern.
func (t Ticket) IsDone() bool {
	return doneStatuses[strings.ToLower(strings.TrimSpace(t.Status))]
}
