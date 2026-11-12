package schedules

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	schedrec "github.com/elentok/gx/schedules"
	"github.com/elentok/gx/server/wire"
)

// DefaultInterval is how often the scheduler checks the stored Schedules.
const DefaultInterval = time.Minute

// Submitter queues a Scheduled run.
type Submitter interface {
	SubmitScheduled(req wire.OneOffRequest, scheduledFor time.Time) (wire.OneOffResult, error)
}

// Scheduler starts a run for each due Schedule. One check does both the timer
// work and the catch-up after a sleep or restart.
type Scheduler struct {
	Store  *schedrec.Store
	State  *State
	Submit Submitter
	// Cancel settles a queued previous run; the server sets it once its ticket operations exist.
	Cancel func(wire.QueueRequest) (wire.QueueResult, error)
	// Notify sends one error line to chat; it has no error result, so a failed send is the chat sink's to log.
	Notify      func(text string)
	TicketStore string
	Log         *slog.Logger
	Now         func() time.Time // nil means time.Now
	Interval    time.Duration    // zero means DefaultInterval
	// Ready is closed once the server may queue runs.
	Ready <-chan struct{}

	checkMu sync.Mutex
}

// Run waits for Ready, checks once, then on each tick until ctx ends.
func (sc *Scheduler) Run(ctx context.Context) {
	select {
	case <-sc.Ready:
	case <-ctx.Done():
		return
	}
	interval := sc.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		sc.Check()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Check reads the Schedules fresh and starts or skips each due one. It is
// serialized because a delete runs one beside the timer's.
func (sc *Scheduler) Check() {
	sc.checkMu.Lock()
	defer sc.checkMu.Unlock()
	items, err := sc.Store.List()
	if err != nil {
		sc.Log.Warn("listing schedules failed", "err", err)
		return
	}
	now := sc.now()
	for _, it := range items {
		sc.checkOne(it.Schedule, it.Registered, now)
	}
	sc.sweepDeleted(items)
}

func (sc *Scheduler) now() time.Time {
	if sc.Now != nil {
		return sc.Now()
	}
	return time.Now()
}

func (sc *Scheduler) checkOne(s schedrec.Schedule, registered bool, now time.Time) {
	k := Key{Project: s.Project, Slug: s.Slug}
	if !sc.State.Enabled(k) {
		return
	}
	if _, ok := sc.State.Baseline(k); !ok {
		sc.record(sc.State.SetBaseline(k, now), k)
		return
	}
	// An edit never starts a run for a time before the save.
	if !sc.record(sc.State.Advance(k, s.UpdatedAt), k) {
		return
	}
	after, _ := sc.State.Baseline(k)
	due := schedrec.LatestDue(s, after, now)
	if due.IsZero() {
		return
	}
	if sc.handleDue(s, k, registered, due, now) {
		sc.record(sc.State.SetBaseline(k, now), k)
	}
}

// handleDue deals with the latest due time. false means keep the baseline so
// a later check retries. A missing project comes before the Late limit.
func (sc *Scheduler) handleDue(s schedrec.Schedule, k Key, registered bool, due, now time.Time) bool {
	if !registered {
		return sc.skipMissingProject(s, k, due, now)
	}
	if limit := time.Duration(s.LateLimitHours * float64(time.Hour)); now.Sub(due) >= limit {
		return sc.record(sc.State.AppendSkip(k, Skip{SetTime: due, Reason: ReasonLate, LoggedAt: now}), k)
	}
	ok, retry := sc.prepare(k, due, now)
	if retry {
		return false
	}
	return !ok || sc.start(s, due)
}

// skipMissingProject logs the skip, then sends one message per set time. A
// restart finds the log entry and sends nothing; a failed send is never retried.
// false means the baseline must stay so a later check retries the log write.
func (sc *Scheduler) skipMissingProject(s schedrec.Schedule, k Key, due, now time.Time) bool {
	if sc.State.Logged(k, due, ReasonProjectNotFound) {
		return true
	}
	if !sc.record(sc.State.AppendSkip(k, Skip{SetTime: due, Reason: ReasonProjectNotFound, LoggedAt: now}), k) {
		return false
	}
	if sc.Notify != nil {
		sc.Notify(fmt.Sprintf("%s: skipped the run for %s, project %q was not found.", s.Name, due.In(now.Location()).Format("15:04"), s.Project))
	}
	return true
}

// start queues the run; false means the baseline must stay so a later check retries.
func (sc *Scheduler) start(s schedrec.Schedule, due time.Time) bool {
	res, err := sc.Submit.SubmitScheduled(sc.runRequest(s), due)
	switch {
	case err != nil:
		sc.Log.Warn("starting a scheduled run failed", "schedule", s.Slug, "err", err)
	case res.Refused && res.Reason == wire.ReasonDuplicateLive:
		return false
	case res.Refused:
		sc.Log.Warn("a scheduled run was refused", "schedule", s.Slug, "reason", res.Reason, "message", res.Message)
	}
	return true
}

// record logs a state write error; it reports whether the write succeeded.
func (sc *Scheduler) record(err error, k Key) bool {
	if err != nil {
		sc.Log.Warn("saving schedule state failed", "schedule", k.String(), "err", err)
	}
	return err == nil
}
