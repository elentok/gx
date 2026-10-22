package ralphloop

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// mergeDeferRetryInterval is how long the worker waits before retrying a merge
// it deferred; a var so tests can shorten it.
var mergeDeferRetryInterval = 2 * time.Second

// mergeLockDir is where an epic's merge lock and marker live.
func mergeLockDir(scratchDir, epic string) (string, error) {
	return MergeLockDir(filepath.Join(scratchDir, epic))
}

// mergeJob is the in-memory handoff from a build goroutine to the merge-queue
// worker: everything mergeCherryPick/markDoneStampingCloseMetadata/
// finishCleanup need that only exists as live-process state (pane/tab/
// sessionID/path), captured at the moment finishIteration discovered ahead >
// 0 commits ready to merge. None of this is persisted — the worker recomputes
// which ticket is eligible to merge from disk each pass (see runMergeQueue);
// this struct only carries what disk can't tell it.
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
// merge-queue worker needs to merge it later. It is not a real failure — every
// caller up the stack (runIteration, reattachIteration, launch's goroutine in
// loop.go) must recognize it via errors.As and route it to the merge queue
// instead of treating it as an iteration error.
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
	Agent           AgentKind
	// Model and Effort are the resolved per-agent config a merged
	// conflict-resolution agent starts under (see RunOptions.Agents); empty
	// omits that one flag from the launch argv.
	Model        string
	Effort       string
	Skill        string
	ScratchDir   string
	WorktreeLock *sync.Mutex
	SmartZone    int
	Gate         *Gate
	Sink         EventSink
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

// runMergeQueue is the merge-queue worker: one goroutine per Run call,
// replacing featureMu as the sole serializer of cherry-pick merges onto the
// feature branch. Each pass picks the lowest-numbered ticket already queued
// in mergeJobs (see pickEligibleMergeJob) — no persisted state of its own —
// and merges it using the in-memory job data captured at build time (see
// mergeJob's doc). It blocks on wake between passes that find nothing queued,
// rather than busy-polling, and exits once done is closed (Run returning).
func runMergeQueue(d Deps, lp mergeQueueParams, mergeJobs map[string]mergeJob, mergeJobsMu *sync.Mutex, wake <-chan struct{}, done <-chan struct{}, mergeResults chan<- outcome) {
	deferredLogged := false // an episode ends when a merge is attempted without deferring
	for {
		select {
		case <-done:
			return
		default:
		}

		job, ok := claimEligibleMergeJob(mergeJobs, mergeJobsMu)
		if !ok {
			select {
			case <-done:
				return
			case <-wake:
			}
			continue
		}

		r := mergeOne(d, lp, job)
		if !r.mergeDeferred {
			deferredLogged = false
		}
		if r.mergeDeferred {
			if !deferredLogged {
				deferredLogged = true
				logMergeDeferred(lp.iterationParamsFor(job))
			}
			// Re-queue and retry on the next tick instead of waiting on the
			// lock; a fresh build's wake just brings the retry forward.
			mergeJobsMu.Lock()
			mergeJobs[job.ticket.Identifier] = job
			mergeJobsMu.Unlock()
			select {
			case <-done:
				return
			case <-wake:
			case <-time.After(mergeDeferRetryInterval):
			}
			continue
		}
		select {
		case mergeResults <- r:
		case <-done:
			return
		}
	}
}

// logMergeDeferred records why a merge is waiting, naming the lock's owner when
// there is one.
func logMergeDeferred(p iterationParams) {
	reason := "merge lock held or conflict pending"
	if dir, err := mergeLockDir(p.ScratchDir, p.FeatureBranch); err == nil {
		if owner, err := ReadMergeLock(dir); err == nil && owner != nil {
			reason = "merge lock held by " + owner.Describe()
		}
	}
	p.logTicketEventReason(string(events.MergeDeferred), "", "", "", reason)
}

// claimEligibleMergeJob picks the lowest-numbered job in mergeJobs (see
// pickEligibleMergeJob) and removes it before returning, so a concurrent
// build goroutine populating a different entry can never see or re-claim the
// same job twice. ok is false if mergeJobs is empty.
func claimEligibleMergeJob(mergeJobs map[string]mergeJob, mergeJobsMu *sync.Mutex) (mergeJob, bool) {
	mergeJobsMu.Lock()
	defer mergeJobsMu.Unlock()
	job, ok := pickEligibleMergeJob(mergeJobs)
	if ok {
		delete(mergeJobs, job.ticket.Identifier)
	}
	return job, ok
}

// pickEligibleMergeJob returns the lowest-numbered ticket among mergeJobs.
// mergeJobs membership alone is what makes a ticket eligible — a build only
// ever adds an entry right after writing MarkBuiltAwaitingMerge, and only the
// worker itself ever removes one (see claimEligibleMergeJob) — so there is
// deliberately no re-read of the ticket's own status/iteration_status/
// blocked_by here: a ticket file is a shared, unlocked plain file (see
// loop.go's launched doc), and an unrelated writer can revert its status out
// from under an already-queued job exactly the way claimNext's own
// launched-map guards against a stray revert causing a reclaim. Trusting
// disk for that would strand the job forever instead. A blocked_by recheck
// was tried and reverted: a type: code-review ticket's Blocked by: is
// synthesized as "every other ticket in the epic" (see
// tickets/status.go's effectiveBlockedBy) and recomputed live, so a review
// that forks findings mid-turn (exactly what a review is for) sees its own
// freshly-written children join that synthesized list the moment they hit
// disk — blocking its own merging on children who in turn can't be claimed
// until their parent (this same ticket) is done. A ticket can only be
// claimed, and thus only ever reach the build that produces a mergeJob, once
// its own blockers already show done, so nothing upstream of it can newly
// unresolve here — recomputing at merge time only re-adds this self-inflicted
// deadlock, so blocked_by is trusted from claim time onward and never
// rechecked.
func pickEligibleMergeJob(mergeJobs map[string]mergeJob) (mergeJob, bool) {
	if len(mergeJobs) == 0 {
		return mergeJob{}, false
	}
	eligible := make([]mergeJob, 0, len(mergeJobs))
	for _, job := range mergeJobs {
		eligible = append(eligible, job)
	}
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].ticket.Number < eligible[j].ticket.Number
	})
	return eligible[0], true
}

// mergeOne runs job's merge — cherry-pick (with conflict resolution if
// needed), mark done, cleanup — the same tail finishIteration ran inline
// before the merge-queue worker existed, now serialized by this worker's own
// single-goroutine-ness instead of featureMu.
func mergeOne(d Deps, lp mergeQueueParams, job mergeJob) outcome {
	p := lp.iterationParamsFor(job)

	// The lock is shared with the human merge command, whose conflicted merge
	// exits still holding it. Never wait on it: pause as deferred, retry next tick.
	lockDir, err := mergeLockDir(p.ScratchDir, p.FeatureBranch)
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
			if _, perr := pause(p.Sink, pauseRequest{
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
