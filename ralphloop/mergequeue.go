package ralphloop

import (
	"errors"
	"fmt"
	"github.com/elentok/gx/ralphloop/ticketops"
	"path/filepath"
	"sync"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// mergeDeferRetryInterval is how long the worker waits before retrying a merge
// it deferred; a var so tests can shorten it.
var mergeDeferRetryInterval = 2 * time.Second

// mergeLockDir is where an epic's merge lock and marker live, under d's lock
// state dir.
func (d Deps) mergeLockDir(scratchDir, epic string) (string, error) {
	return MergeLockDirIn(d.MergeLockStateDir, filepath.Join(scratchDir, epic))
}

// mergeJob is the in-memory handoff from a build goroutine to the merge-queue
// worker: everything mergeCherryPick/markDoneStampingCloseMetadata/
// finishCleanup need that only exists as live-process state (pane/tab/
// sessionID/path), captured at the moment finishIteration discovered ahead >
// 0 commits ready to merge. None of this is persisted — the worker recomputes
// which ticket is eligible to merge from disk; this struct only carries what
// disk can't tell it.
type mergeJob struct {
	ticket    tickets.Ticket
	base      string
	branch    string
	sessionID string
	pane      string
	path      string // iteration worktree path, for the string(events.CherryPicked) log call
}

// builtAwaitingMergeError is finishIteration's signal that a build finished
// with commits ready to merge (ahead > 0): iteration_status: finished has
// already been written (see MarkBuiltAwaitingMerge), and job carries what the
// merge step needs to merge it later. It is not a real failure — every
// caller up the stack must recognize it via errors.As and route it to the
// merge step instead of treating it as an iteration error.
type builtAwaitingMergeError struct {
	job mergeJob
}

func (e *builtAwaitingMergeError) Error() string {
	return fmt.Sprintf("ticket %s built, queued to merge", e.job.ticket.Identifier)
}

// mergeQueueParams bundles the merge-queue worker's fixed-for-the-run inputs —
// everything mergeCherryPick/markDoneStampingCloseMetadata/finishCleanup need
// beyond what an individual mergeJob itself carries. Mirrors iterationParams
// minus the per-ticket fields (Ticket, Report) that mergeJob/the call site
// supply instead.
type mergeQueueParams struct {
	WorkspaceID     string
	RepoDir         string
	WorktreeDir     string
	FeatureWorktree string
	FeatureBranch   string
	Agent           agent.AgentKind
	// Model and Effort are the resolved per-agent config a merged
	// conflict-resolution agent starts under (see agent.ResolveTier); empty
	// omits that one flag from the launch argv.
	Model        string
	Effort       string
	Skill        string
	ScratchDir   string
	WorktreeLock *sync.Mutex
	SmartZone    int
	Gate         *Gate
	Sink         runlog.EventSink
}

// iterationParamsFor rebuilds the iterationParams mergeCherryPick/
// markDoneStampingCloseMetadata expect, scoped to job's ticket — the same
// shape launch's own construction in loop.go uses, minus FeatureLock (gone
// with the mutex this worker replaces).
func (lp mergeQueueParams) iterationParamsFor(job mergeJob) iterationParams {
	return iterationParams{
		WorkspaceID:     lp.WorkspaceID,
		RepoDir:         lp.RepoDir,
		WorktreeDir:     lp.WorktreeDir,
		FeatureWorktree: lp.FeatureWorktree,
		FeatureBranch:   lp.FeatureBranch,
		Agent:           lp.Agent,
		Model:           lp.Model,
		Effort:          lp.Effort,
		Skill:           lp.Skill,
		Ticket:          job.ticket,
		ScratchDir:      lp.ScratchDir,
		WorktreeLock:    lp.WorktreeLock,
		SmartZone:       lp.SmartZone,
		Gate:            lp.Gate,
		Sink:            lp.Sink,
	}
}

// logMergeDeferred records why a merge is waiting, naming the lock's owner when
// there is one.
func logMergeDeferred(d Deps, p iterationParams) {
	reason := "merge lock held or conflict pending"
	if dir, err := d.mergeLockDir(p.ScratchDir, p.FeatureBranch); err == nil {
		if owner, err := ReadMergeLock(dir); err == nil && owner != nil {
			reason = "merge lock held by " + owner.Describe()
		}
	}
	p.logTicketEventReason(string(events.MergeDeferred), "", "", "", reason)
}

// mergeOne runs job's merge — cherry-pick (with conflict resolution if
// needed), mark done, cleanup — the same tail finishIteration ran inline
// before the merge-queue worker existed, now serialized by this worker's own
// single-goroutine-ness instead of featureMu.
func mergeOne(d Deps, lp mergeQueueParams, job mergeJob) outcome {
	p := lp.iterationParamsFor(job)

	// The lock is shared with the human merge command, whose conflicted merge
	// exits still holding it. Never wait on it: pause as deferred, retry next tick.
	lockDir, err := d.mergeLockDir(p.ScratchDir, p.FeatureBranch)
	if err != nil {
		return outcome{ticket: job.ticket, err: err}
	}
	// The pre-pick HEAD lets a restart finish this merging if gx stops
	// after git does (see recoverInterruptedMerging).
	head, _ := d.RevParse(p.FeatureWorktree, "HEAD")
	if err := AcquireMergeLockAt(lockDir, p.FeatureBranch, job.ticket.Identifier, head); err != nil {
		if errors.Is(err, ErrMergeLocked) {
			return outcome{ticket: job.ticket, mergeDeferred: true}
		}
		return outcome{ticket: job.ticket, err: fmt.Errorf("taking merge lock: %w", err)}
	}
	defer ReleaseMergeLock(lockDir)

	mergedSHA, err := mergeCherryPick(d, p, job.base, job.branch, job.sessionID, job.pane)
	if err != nil {
		if errors.Is(err, errMergeDeferred) {
			return outcome{ticket: job.ticket, mergeDeferred: true}
		}
		if errors.Is(err, errConflictResolutionUnresolved) {
			// Already paused paused (repair) on the conflict-resolution child
			// ticket (see errConflictResolutionUnresolved's doc comment);
			// the parent iteration ticket stays claimed, and its worktree/
			// tab/branch survive for a later orphaned-claim reconcile to
			// retry — not a failure of this merge itself.
			return outcome{ticket: job.ticket, pausedOnChild: true, childReason: err.Error()}
		}
		var amb *ambiguousMergeError
		if errors.As(err, &amb) {
			// Paused, not failed: callers then see it via the pause check / the ticket.
			if _, perr := ticketops.PauseTicket(p.Sink, ticketops.PauseRequest{
				ScratchDir: p.ScratchDir, EpicName: p.FeatureBranch, Ticket: p.Ticket.Identifier, Path: p.Ticket.Path,
				Type: events.Paused, Kind: events.AmbiguousMerge, Reason: amb.Error(),
				Repair: schema.FaultState{Label: iterLabel(p.FeatureBranch, p.Ticket.Identifier), Branch: job.branch, Worktree: job.path},
			}); perr != nil {
				return outcome{ticket: job.ticket, err: perr}
			}
			return outcome{ticket: job.ticket}
		}
		return outcome{ticket: job.ticket, err: err}
	}
	p.logTicketEventSHA(string(events.CherryPicked), job.pane, job.sessionID, job.path, "", mergedSHA)

	if err := markDoneStampingCloseMetadata(d, p, job.path, job.sessionID); err != nil {
		return outcome{ticket: job.ticket, err: fmt.Errorf("marking ticket done: %w", err)}
	}

	// MarkBuiltAwaitingMerge's iteration_status: finished was only ever a
	// transient signal for this worker's own eligibility scan — its job is
	// done the moment status: done is stamped above, so it's cleared here
	// rather than left stamped permanently on every merged ticket (unlike
	// the commitless path's own iteration_status: finished, which is an
	// agent's honest self-report worth keeping as a historical record, this
	// one is gx's own bookkeeping with nothing left to record once merged).
	if err := schema.ClearIterationStatus(job.ticket.Path); err != nil {
		return outcome{ticket: job.ticket, err: fmt.Errorf("clearing iteration_status after merging: %w", err)}
	}

	if err := finishCleanup(d, p.WorktreeLock, p.RepoDir, p.FeatureWorktree, job.path, job.branch, iterationSession(p, job.pane), true); err != nil {
		return outcome{ticket: job.ticket, err: err}
	}

	return outcome{ticket: job.ticket}
}
