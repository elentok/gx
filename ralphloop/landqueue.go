package ralphloop

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// landJob is the in-memory handoff from a build goroutine to the land-queue
// worker: everything landCherryPick/markDoneStampingCloseMetadata/
// finishCleanup need that only exists as live-process state (pane/tab/
// sessionID/path), captured at the moment finishIteration discovered ahead >
// 0 commits ready to land. None of this is persisted — the worker recomputes
// which ticket is eligible to land from disk each pass (see runLandQueue);
// this struct only carries what disk can't tell it.
type landJob struct {
	ticket    tickets.Ticket
	base      string
	branch    string
	sessionID string
	pane      string
	tab       string
	path      string // iteration worktree path, for the eventCherryPicked log call
}

// builtAwaitingLandError is finishIteration's signal that a build finished
// with commits ready to land (ahead > 0): iteration_status: finished has
// already been written (see MarkBuiltAwaitingLand), and job carries what the
// land-queue worker needs to land it later. It is not a real failure — every
// caller up the stack (runIteration, reattachIteration, launch's goroutine in
// loop.go) must recognize it via errors.As and route it to the land queue
// instead of treating it as an iteration error.
type builtAwaitingLandError struct {
	job landJob
}

func (e *builtAwaitingLandError) Error() string {
	return fmt.Sprintf("ticket %s built, queued to land", e.job.ticket.Identifier)
}

// landQueueParams bundles the land-queue worker's fixed-for-the-run inputs —
// everything landCherryPick/markDoneStampingCloseMetadata/finishCleanup need
// beyond what an individual landJob itself carries. Mirrors iterationParams
// minus the per-ticket fields (Ticket, Report) that landJob/the call site
// supply instead.
type landQueueParams struct {
	WorkspaceID     string
	RepoDir         string
	WorktreeDir     string
	FeatureWorktree string
	FeatureBranch   string
	Agent           AgentKind
	Skill           string
	ScratchDir      string
	WorktreeLock    *sync.Mutex
	SmartZone       int
	Gate            *Gate
	Sink            EventSink
}

// iterationParamsFor rebuilds the iterationParams landCherryPick/
// markDoneStampingCloseMetadata expect, scoped to job's ticket — the same
// shape launch's own construction in loop.go uses, minus FeatureLock (gone
// with the mutex this worker replaces).
func (lp landQueueParams) iterationParamsFor(job landJob) iterationParams {
	return iterationParams{
		WorkspaceID:     lp.WorkspaceID,
		RepoDir:         lp.RepoDir,
		WorktreeDir:     lp.WorktreeDir,
		FeatureWorktree: lp.FeatureWorktree,
		FeatureBranch:   lp.FeatureBranch,
		Agent:           lp.Agent,
		Skill:           lp.Skill,
		Ticket:          job.ticket,
		ScratchDir:      lp.ScratchDir,
		WorktreeLock:    lp.WorktreeLock,
		SmartZone:       lp.SmartZone,
		Gate:            lp.Gate,
		Sink:            lp.Sink,
	}
}

// runLandQueue is the land-queue worker: one goroutine per Run call,
// replacing featureMu as the sole serializer of cherry-pick landings onto the
// feature branch. Each pass picks the lowest-numbered ticket already queued
// in landJobs (see pickEligibleLandJob) — no persisted state of its own —
// and lands it using the in-memory job data captured at build time (see
// landJob's doc). It blocks on wake between passes that find nothing queued,
// rather than busy-polling, and exits once done is closed (Run returning).
func runLandQueue(d Deps, lp landQueueParams, landJobs map[string]landJob, landJobsMu *sync.Mutex, wake <-chan struct{}, done <-chan struct{}, landResults chan<- outcome) {
	for {
		select {
		case <-done:
			return
		default:
		}

		job, ok := claimEligibleLandJob(landJobs, landJobsMu)
		if !ok {
			select {
			case <-done:
				return
			case <-wake:
			}
			continue
		}

		r := landOne(d, lp, job)
		select {
		case landResults <- r:
		case <-done:
			return
		}
	}
}

// claimEligibleLandJob picks the lowest-numbered job in landJobs (see
// pickEligibleLandJob) and removes it before returning, so a concurrent
// build goroutine populating a different entry can never see or re-claim the
// same job twice. ok is false if landJobs is empty.
func claimEligibleLandJob(landJobs map[string]landJob, landJobsMu *sync.Mutex) (landJob, bool) {
	landJobsMu.Lock()
	defer landJobsMu.Unlock()
	job, ok := pickEligibleLandJob(landJobs)
	if ok {
		delete(landJobs, job.ticket.Identifier)
	}
	return job, ok
}

// pickEligibleLandJob returns the lowest-numbered ticket among landJobs.
// landJobs membership alone is what makes a ticket eligible — a build only
// ever adds an entry right after writing MarkBuiltAwaitingLand, and only the
// worker itself ever removes one (see claimEligibleLandJob) — so there is
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
// disk — blocking its own landing on children who in turn can't be claimed
// until their parent (this same ticket) is done. A ticket can only be
// claimed, and thus only ever reach the build that produces a landJob, once
// its own blockers already show done, so nothing upstream of it can newly
// unresolve here — recomputing at land time only re-adds this self-inflicted
// deadlock, so blocked_by is trusted from claim time onward and never
// rechecked.
func pickEligibleLandJob(landJobs map[string]landJob) (landJob, bool) {
	if len(landJobs) == 0 {
		return landJob{}, false
	}
	eligible := make([]landJob, 0, len(landJobs))
	for _, job := range landJobs {
		eligible = append(eligible, job)
	}
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].ticket.Number < eligible[j].ticket.Number
	})
	return eligible[0], true
}

// landOne runs job's land — cherry-pick (with conflict resolution if
// needed), mark done, cleanup — the same tail finishIteration ran inline
// before the land-queue worker existed, now serialized by this worker's own
// single-goroutine-ness instead of featureMu.
func landOne(d Deps, lp landQueueParams, job landJob) outcome {
	p := lp.iterationParamsFor(job)

	landedSHA, err := landCherryPick(d, p, job.base, job.branch, job.sessionID, job.pane, job.tab)
	if err != nil {
		if errors.Is(err, errConflictResolutionUnresolved) {
			// Already parked needs-repair on the conflict-resolution child
			// ticket (see errConflictResolutionUnresolved's doc comment);
			// the parent iteration ticket stays claimed, and its worktree/
			// tab/branch survive for a later orphaned-claim reconcile to
			// retry — not a failure of this land itself.
			return outcome{ticket: job.ticket}
		}
		return outcome{ticket: job.ticket, err: err}
	}
	p.logTicketEventSHA(eventCherryPicked, job.pane, job.tab, job.sessionID, job.path, "", landedSHA)

	if err := markDoneStampingCloseMetadata(d, p, job.path, job.sessionID); err != nil {
		return outcome{ticket: job.ticket, err: fmt.Errorf("marking ticket done: %w", err)}
	}

	// MarkBuiltAwaitingLand's iteration_status: finished was only ever a
	// transient signal for this worker's own eligibility scan — its job is
	// done the moment status: done is stamped above, so it's cleared here
	// rather than left stamped permanently on every landed ticket (unlike
	// the commitless path's own iteration_status: finished, which is an
	// agent's honest self-report worth keeping as a historical record, this
	// one is gx's own bookkeeping with nothing left to record once landed).
	if err := schema.ClearIterationStatus(job.ticket.Path); err != nil {
		return outcome{ticket: job.ticket, err: fmt.Errorf("clearing iteration_status after landing: %w", err)}
	}

	if err := finishCleanup(d, p.WorktreeLock, p.RepoDir, p.FeatureWorktree, job.path, job.branch, job.tab, true); err != nil {
		return outcome{ticket: job.ticket, err: err}
	}

	return outcome{ticket: job.ticket}
}
