package schedules

import (
	"path/filepath"

	schedrec "github.com/elentok/gx/schedules"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// sweepDeleted tidies this machine's side of every Schedule that has state but
// no file. items comes from a successful list, so a failed read never gets here.
func (sc *Scheduler) sweepDeleted(items []schedrec.Item) {
	present := make(map[Key]bool, len(items))
	for _, it := range items {
		present[Key{Project: it.Schedule.Project, Slug: it.Schedule.Slug}] = true
	}
	for _, k := range sc.State.Keys() {
		if present[k] {
			continue
		}
		if !sc.cancelQueued(k) {
			continue
		}
		sc.record(sc.State.Delete(k), k)
	}
}

// cancelQueued cancels this machine's queued never-started runs of the Schedule.
// false means a cancel failed and the state entries must stay for the next check.
func (sc *Scheduler) cancelQueued(k Key) bool {
	live, _, err := tickets.LoadOneOffs(filepath.Join(sc.TicketStore, k.Project))
	if err != nil {
		sc.Log.Warn("finding a deleted schedule's runs failed", "schedule", k.String(), "err", err)
		return false
	}
	key := sc.dedupeKey(k)
	ok := true
	for _, t := range live.Tickets {
		if t.Unique != key || t.Status != "open" || !t.ClaimedAt.IsZero() {
			continue
		}
		addr := tickets.Address{Project: k.Project, Epic: tickets.OneOffsEpic, ID: t.DisplayNumber()}.String()
		res, err := sc.Cancel(wire.QueueRequest{Address: addr, Stop: true})
		if err != nil || res.Refused {
			sc.Log.Warn("cancelling a deleted schedule's run failed", "schedule", k.String(), "address", addr, "err", err)
			ok = false
		}
	}
	return ok
}
