package schedules

import (
	"context"
	"log/slog"
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
	Store    *schedrec.Store
	State    *State
	Submit   Submitter
	Log      *slog.Logger
	Now      func() time.Time // nil means time.Now
	Interval time.Duration    // zero means DefaultInterval
	// Ready is closed once the server may queue runs.
	Ready <-chan struct{}
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

// Check reads the Schedules fresh and starts or skips each due one.
func (sc *Scheduler) Check() {
	items, err := sc.Store.List()
	if err != nil {
		sc.Log.Warn("listing schedules failed", "err", err)
		return
	}
	now := sc.now()
	for _, it := range items {
		if it.Registered {
			sc.checkOne(it.Schedule, now)
		}
	}
}

func (sc *Scheduler) now() time.Time {
	if sc.Now != nil {
		return sc.Now()
	}
	return time.Now()
}

func (sc *Scheduler) checkOne(s schedrec.Schedule, now time.Time) {
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
	limit := time.Duration(s.LateLimitHours * float64(time.Hour))
	if now.Sub(due) >= limit {
		sk := Skip{SetTime: due, Reason: ReasonLate, LoggedAt: now}
		if !sc.record(sc.State.AppendSkip(k, sk), k) {
			return
		}
	} else if !sc.start(s, due) {
		return
	}
	sc.record(sc.State.SetBaseline(k, now), k)
}

// start queues the run; false means the baseline must stay so a later check retries.
func (sc *Scheduler) start(s schedrec.Schedule, due time.Time) bool {
	res, err := sc.Submit.SubmitScheduled(wire.OneOffRequest{
		Prompt:  Prompt(s.Name, s.Prompt),
		Project: s.Project,
		Agent:   s.Harness,
		Name:    s.Slug,
		Unique:  "schedule:" + s.Project + "/" + s.Slug + "@" + sc.State.MachineID(),
	}, due)
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
