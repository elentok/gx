package schedules

import (
	"fmt"
	"time"

	schedrec "github.com/elentok/gx/schedules"
	"github.com/elentok/gx/server/wire"
)

// runRequest is the submit of one run of the Schedule, timed or Run now.
func (sc *Scheduler) runRequest(s schedrec.Schedule) wire.OneOffRequest {
	return wire.OneOffRequest{
		Prompt:  Prompt(s.Name, s.Prompt),
		Project: s.Project,
		Agent:   s.Harness,
		Name:    s.Slug,
		Unique:  sc.dedupeKey(Key{Project: s.Project, Slug: s.Slug}),
	}
}

func runNowRefusal(reason, message, attention string) wire.ScheduleRunNowResult {
	return wire.ScheduleRunNowResult{Refused: true, Reason: reason, Message: message, AttentionAddress: attention}
}

// RunNow starts one run now, under the timed rule: a queued never-started
// previous run is replaced, any other live one refuses. A refusal logs no
// skip. It ignores the switch and leaves the baseline alone.
func (sc *Scheduler) RunNow(s schedrec.Schedule) (wire.ScheduleRunNowResult, error) {
	sc.checkMu.Lock()
	defer sc.checkMu.Unlock()
	k := Key{Project: s.Project, Slug: s.Slug}
	d, err := sc.Decision(k)
	if err != nil {
		return wire.ScheduleRunNowResult{}, err
	}
	if d.Action == ActionReplace {
		res, cerr := sc.Cancel(wire.QueueRequest{Address: d.Address, Stop: true})
		if cerr != nil || res.Refused {
			// It may have started in between; look again.
			if d, err = sc.Decision(k); err != nil {
				return wire.ScheduleRunNowResult{}, err
			}
			if d.Action != ActionSkip {
				if cerr == nil {
					cerr = fmt.Errorf("cancelling the previous run was refused (%s): %s", res.Reason, res.Message)
				}
				return wire.ScheduleRunNowResult{}, cerr
			}
		}
	}
	if d.Action == ActionSkip {
		res := runNowRefusal(d.Reason, "the previous run of "+s.Name+" is live: "+d.Address, "")
		if d.Reason == ReasonPreviousRunNeedsAttention {
			res.AttentionAddress = d.Address
		}
		return res, nil
	}
	out, err := sc.Submit.SubmitScheduled(sc.runRequest(s), time.Time{})
	switch {
	case err != nil:
		return wire.ScheduleRunNowResult{}, err
	case out.Refused:
		return runNowRefusal(out.Reason, out.Message, ""), nil
	}
	return wire.ScheduleRunNowResult{Address: out.Address, Status: out.Status}, nil
}
