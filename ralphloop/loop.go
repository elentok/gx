package ralphloop

import (
	"cmp"
	"strings"

	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
)

// ticketTrailerKey is the commit-message trailer mergeCherryPick stamps onto
// every merged commit (see Deps.AppendTrailer) and verify's last-resort
// fallback searches for (Deps.TrailerCommitExists).
const ticketTrailerKey = "Ralph-Loop-Ticket"

// TicketTrailerKey is ticketTrailerKey for the repair verbs.
const TicketTrailerKey = ticketTrailerKey

// tokensTrailerKey and elapsedTrailerKey are the commit-message trailers
// mergeCherryPick stamps alongside ticketTrailerKey whenever the merging
// session's own metrics are available (see writeMergedMetrics) — the same
// actual_context_window/elapsed_time values written to the ticket's
// frontmatter, surfaced on the merged commit too.
const (
	tokensTrailerKey  = "Ralph-Loop-Tokens"
	elapsedTrailerKey = "Ralph-Loop-Elapsed"
	costTrailerKey    = "Ralph-Loop-Cost"
)

// ticketTrailerValue builds ticketTrailerKey's value, scoped to epicName: a
// bare ticket identifier repeats across every epic (each restarts numbering
// from 01), and TrailerCommitExists searches the whole repo history, not just
// commits under this epic. An unscoped value can match a same-numbered
// ticket from a completely unrelated epic merged long ago, misreporting a
// genuinely unmerged ticket as already present.
func ticketTrailerValue(epicName, identifier string) string {
	return epicName + "/" + identifier
}

// defaultMaxParallel is how many iterations run concurrently when no
// MaxParallel is configured.
const defaultMaxParallel = 2

// ticketAddress is the canonical "project:epic/NN" address of the ticket
// file at ticketPath (<root>/<epic>/issues/NN-<slug>.md), the only form
// iteration prompts hand to agents.
func ticketAddress(t tickets.Ticket) string {
	a, _ := tickets.AddressOfPath(t.Path, t.Identifier)
	return a.String()
}

// outcome is one iteration's or one merge's result.
type outcome struct {
	ticket tickets.Ticket
	err    error
	// built is true when this outcome is a build that finished with commits
	// ready to merge (ahead > 0): iteration_status: finished is already
	// written.
	built bool
	// pausedOnChild is true when this outcome is a merge outcome whose
	// cherry-pick hit errConflictResolutionUnresolved: the ticket was paused
	// (repair) on a conflict-resolution child, not merged — it stays claimed on
	// disk. err is nil on this path (pausing already handled the failure).
	pausedOnChild bool
	// childReason names the conflict-resolution child and why it failed, on a
	// pausedOnChild outcome; the server pauses the parent with it.
	childReason string
	// mergeDeferred is set by mergeOne when the merge lock was held or a human's
	// conflict marker is pending.
	mergeDeferred bool
}

// scanDecisions builds one claimNext pass's scheduler-scan log line: epic's
// tickets, in file order, each labeled with why the scheduler did or didn't
// claim it this pass. claimed is the zero-value Ticket when this pass found
// nothing to claim.
func scanDecisions(epic tickets.Epic, scope RunScope, frontier []tickets.Ticket, claimed tickets.Ticket) []runlog.ScanDecision {
	inFrontier := make(map[string]bool, len(frontier))
	for _, t := range frontier {
		inFrontier[t.Path] = true
	}

	decisions := make([]runlog.ScanDecision, 0, len(epic.Tickets))
	for _, t := range epic.Tickets {
		isClaimed := claimed.Path != "" && t.Path == claimed.Path
		decisions = append(decisions, TicketVerdict(epic, scope, t, isClaimed, inFrontier[t.Path]))
	}
	return decisions
}

// TicketVerdict is the scheduler's single per-ticket eligibility decision:
// why t was or wasn't claimed this pass, with the reason a scan line reports.
// The first matching rule wins, so the order is the precedence.
func TicketVerdict(epic tickets.Epic, scope RunScope, t tickets.Ticket, claimed, inFrontier bool) runlog.ScanDecision {
	status := epic.RenderedStatus(t)
	d := runlog.ScanDecision{Ticket: t.Identifier, Status: status.Word()}
	switch {
	case claimed:
		d.Decision = runlog.DecisionClaimed
	case !scope.Contains(t, epic):
		d.Decision = runlog.DecisionOutOfScope
	case status.Terminal():
		d.Decision = runlog.DecisionDone
	case status == tickets.StatusError:
		d.Decision = runlog.DecisionError
		d.Reason = cmp.Or(t.ReadErr, t.GraphErr, t.BlockedByErr, "unrecognized status "+t.Status)
	case epic.IsPaused(t):
		d.Decision = runlog.DecisionStalled
	case inFrontier:
		d.Decision = runlog.DecisionFrontier
	case status == tickets.StatusBlocked:
		d.Decision = runlog.DecisionBlocked
		d.Reason = strings.Join(epic.UnresolvedBlockers(t), ", ")
	default:
		d.Decision = runlog.DecisionUnclaimed
	}
	return d
}

// AllDone is allDone for the server's runner.
func AllDone(e tickets.Epic) bool { return allDone(e) }

// allDone reports whether every ticket in e is done — the run's one exit
// condition. A stalled ticket (see tickets.Epic.IsPaused) is deliberately not an
// exit condition: the run pauses on it instead, so the agent's question is
// still answerable in its own pane.
func allDone(e tickets.Epic) bool {
	if len(e.Tickets) == 0 {
		return false
	}
	for _, t := range e.Tickets {
		if !e.RenderedStatus(t).Terminal() {
			return false
		}
	}
	return true
}

// loadNamedEpic loads scratchDir and returns the epic named name, or nil if
// no such epic exists.
func loadNamedEpic(scratchDir, name string) (*tickets.Epic, error) {
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		return nil, err
	}
	for i := range epics {
		if epics[i].Name == name {
			return &epics[i], nil
		}
	}
	return nil, nil
}
