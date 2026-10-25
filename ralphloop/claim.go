package ralphloop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/fsutil"
	"github.com/elentok/gx/storecommit"
	"github.com/elentok/gx/tickets/schema"
)

// Claim writes status: claimed into the ticket file at path, clearing any
// iteration_status left over from a prior claim of this ticket — a stale
// self-report must not survive into a fresh claim. It also clears pause_kind:
// Claim is the single choke point every reopen path funnels through (auto
// resume, a hand-edited status flip back to open, the queue tab's
// suggested-actions menu), so clearing it only in ResumeTicket would miss
// the hand-edited path and leave a stale pause_kind for a future pause to
// misread. It also demotes any "## Needs Repair" section into a dated "##
// Comments" sub-entry: per the retirement principle (only a person reads a
// fault section, so gx — not the next agent — retires it), and claim is the
// moment a ticket starts a new life. Doing this here rather than at reattach
// means a ticket that reattaches several times within one claim never
// re-fires the demotion.
func Claim(path string) error { return ClaimWithBase(path, "") }

// ClaimWithBase is Claim that also stamps resolvedBase (ref + SHA) on the
// ticket; "" leaves the previous stamp alone.
func ClaimWithBase(path, resolvedBase string) error {
	return updateTicketWithBody(path, func(t *schema.Ticket, body *string) {
		t.Status = schema.StatusClaimed
		// Stamped here as well because a re-claim of an already-claimed ticket
		// is not a status change.
		t.ClaimedAt = time.Now().UTC()
		t.DoneAt = time.Time{}
		if resolvedBase != "" {
			t.ResolvedBase = resolvedBase
		}
		t.IterationStatus = ""
		t.IterationFinishedAt = time.Time{}
		t.PauseKind = ""
		t.PauseReason = ""
		*body = schema.DemotePauseSections(*body, time.Now())
	})
}

// MarkDone writes status: done into the ticket file at path.
func MarkDone(path string) error {
	return SetStatus(path, "done")
}

// MarkBuiltAwaitingMerge writes iteration_status: finished into the ticket
// file at path, leaving Status: claimed untouched — gx's own signal that a
// build finished with commits ready to merge, queued behind the merge-queue
// worker rather than merged inline. This widens IterationStatusFinished's
// meaning beyond an agent's own commitless self-report (see
// schema.IterationStatus's doc); RenderedStatus keys only off Status, so a
// claimed ticket bearing this never leaks into the frontier while it waits.
func MarkBuiltAwaitingMerge(path string) error {
	return updateTicket(path, func(t *schema.Ticket) {
		t.IterationStatus = schema.IterationStatusFinished
	})
}

// markAsking writes status: paused (answer) into the ticket file at path,
// stamping pauseKind so a later reader can tell which of the three producers
// paused it.
func markAsking(path string, pauseKind schema.PauseKind) error {
	return pauseWrite(path, false, func(t *schema.Ticket, _ *string) {
		t.Status = schema.StatusPaused
		t.PauseKind = ""
		t.PauseReason = schema.PausedReason(string(pauseKind), schema.PauseClassAnswer)
	})
}

// ErrPauseDone is returned by a pause write on a ticket that is already done.
var ErrPauseDone = errors.New("refusing to pause a done ticket")

// pauseWrite is the shared pause write: a done ticket is refused and left
// untouched, unless repauseDone says the done itself is what paused for repair. The
// check sits under the ticket lock so a merge racing the pause cannot slip a
// done between check and write.
func pauseWrite(path string, repauseDone bool, mutate func(*schema.Ticket, *string)) error {
	return rewriteTicket(path, false, func(t *schema.Ticket, body *string) error {
		if t.Status == schema.StatusDone && !repauseDone {
			return ErrPauseDone
		}
		mutate(t, body)
		return nil
	})
}

// markRepairWithReason writes status: paused (repair) into the ticket file,
// stamps pauseKind (the same kind the pause event carries), and appends a "## Needs Repair" section built by schema.FormatFaultBody
// (summary/detail split from reason, plus state rendered best-effort), so the
// full failure is readable by opening the ticket file even when the live
// UI's status subtext truncates it. Fails without writing anything if reason
// is empty — see FormatFaultBody's write-conditional validation.
func markRepairWithReason(path, reason string, pauseKind schema.PauseKind, state schema.FaultState, repauseDone bool) error {
	section, err := schema.FormatFaultBody(reason, state)
	if err != nil {
		return err
	}
	return pauseWrite(path, repauseDone, func(t *schema.Ticket, body *string) {
		t.Status = schema.StatusPaused
		t.PauseKind = ""
		t.PauseReason = schema.PausedReason(string(pauseKind), schema.PauseClassRepair)
		*body += section
	})
}

// MarkDoneWithMetadata marks a ticket done and records the closing
// iteration's compaction count, in a single atomic write. It leaves
// actual_context_window alone: writeMergedMetrics owns that field and stamps
// the transcript's peak, which the last turn's occupancy under-reports once a
// session has compacted. sessionID is accepted for caller compatibility but
// not persisted: per .scratch/ticket-frontmatter/spec.md, the Session field
// is dropped from frontmatter entirely.
func MarkDoneWithMetadata(path string, compactions int, sessionID string) error {
	return updateTicket(path, func(t *schema.Ticket) {
		t.Status = schema.StatusDone
		t.Compactions = compactions
	})
}

// AppendSessionID appends sessionID to the ticket's session_ids frontmatter
// list, leaving every existing entry untouched — every fresh launch or
// reattach gets its own entry so a ticket resumed across multiple agent
// sessions keeps all of them available for retrospect, unlike the
// single-value Session field MarkDoneWithMetadata's doc comment describes as
// dropped.
func AppendSessionID(path, sessionID string) error {
	return updateTicket(path, func(t *schema.Ticket) {
		t.SessionIDs = append(t.SessionIDs, sessionID)
	})
}

// SetStatus rewrites (or, if the ticket's status is unset, sets for the
// first time) a ticket file's frontmatter status field to value, leaving
// the rest of the file - other frontmatter fields and the body - unchanged.
func SetStatus(path, value string) error {
	return updateTicket(path, func(t *schema.Ticket) {
		t.Status = schema.Status(value)
	})
}

// CancelWithResolution cancels the ticket at path and records why under its
// "## Resolution" section.
func CancelWithResolution(path, resolution string) error {
	return updateTicketWithBody(path, func(t *schema.Ticket, body *string) {
		t.Status = schema.StatusCancelled
		*body = schema.SetSection(*body, "Resolution", resolution)
	})
}

// updateTicket reads path's frontmatter ticket, applies mutate to it, and
// writes the result back via schema.MarshalTicket, leaving the body
// untouched. The write goes through writeFileAtomic so a concurrent reader
// (the scheduler re-scanning the frontier while another goroutine
// claims/completes a ticket) always sees either the old or the new content
// in full, never a torn/truncated write from an in-place os.WriteFile.
func updateTicket(path string, mutate func(*schema.Ticket)) error {
	return updateTicketWithBody(path, func(t *schema.Ticket, _ *string) {
		mutate(t)
	})
}

// updateTicketWithBody is updateTicket's variant for mutations that also
// need to rewrite the ticket's markdown body (e.g. appending a note),
// leaving the same atomic-write/torn-read guarantees.
func updateTicketWithBody(path string, mutate func(*schema.Ticket, *string)) error {
	return rewriteTicket(path, false, func(t *schema.Ticket, body *string) error {
		mutate(t, body)
		return nil
	})
}

// MarkChildDone differs from MarkDone for a conflict-resolution child gx
// closes itself after the agent exits: MarkDone refuses a ticket that is
// invalid on disk, but the agent may have hand-edited the child into an
// invalid status (e.g. "resolved"). gx owns that field, so MarkChildDone
// repairs status to done instead of failing the close and pausing the parent.
// Any other invalid field is still an error and nothing is written, since
// writing it back would make the next Load fail on the child. It also records
// the resolver's sessionID in the same write, because the agent's own edits
// can leave the child invalid until this repair.
func MarkChildDone(path, sessionID string) error {
	return rewriteTicket(path, true, func(t *schema.Ticket, _ *string) error {
		t.Status = schema.StatusDone
		t.SessionIDs = append(t.SessionIDs, sessionID)
		return nil
	})
}

// rewriteTicket applies mutate under the ticket lock and writes the result
// atomically; an error from mutate writes nothing. With repair, an invalid
// status is tolerated on read (mutate is expected to fix it) and the ticket is
// validated after mutate; otherwise the ticket must already be valid.
func rewriteTicket(path string, repair bool, mutate func(*schema.Ticket, *string) error) error {
	unlock, err := schema.LockTicket(path)
	if err != nil {
		return err
	}
	defer unlock()
	return rewriteTicketLocked(path, repair, mutate)
}

// rewriteTicketLocked is rewriteTicket for a caller that already holds the
// ticket lock.
func rewriteTicketLocked(path string, repair bool, mutate func(*schema.Ticket, *string) error) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var t schema.Ticket
	if repair {
		r, err := schema.ParseTicketRaw(string(raw), path)
		if err != nil {
			return fmt.Errorf("reading ticket %s: %w", path, err)
		}
		t = r.Ticket
	} else if t, err = schema.ParseTicketFromRaw(string(raw), path); err != nil {
		return fmt.Errorf("reading ticket %s: %w", path, err)
	}
	body := schema.ParseBody(string(raw))

	before := t.Status
	if err := mutate(&t, &body); err != nil {
		return err
	}
	t.StampStatusChange(before, time.Now().UTC())

	if repair {
		if err := schema.Validate(t); err != nil {
			return fmt.Errorf("repairing ticket %s: still invalid: %w", path, err)
		}
	}

	out, err := schema.MarshalTicket(t, body)
	if err != nil {
		return fmt.Errorf("marshaling ticket %s: %w", path, err)
	}
	if err := fsutil.WriteFileAtomic(path, out); err != nil {
		return err
	}
	commitStatusChange(path, t)
	return nil
}

// commitStatusChange commits the store right away when t just reached done,
// cancelled or a pause status. Best-effort: a failed commit never fails the
// ticket write, and the debounced loop picks the change up anyway.
func commitStatusChange(path string, t schema.Ticket) {
	switch t.Status {
	case schema.StatusDone, schema.StatusCancelled, schema.StatusPaused:
	default:
		return
	}
	epic := filepath.Base(filepath.Dir(filepath.Dir(path)))
	_ = storecommit.Immediate(path, fmt.Sprintf("%s/%s: %s", epic, t.ID, t.Status))
}
