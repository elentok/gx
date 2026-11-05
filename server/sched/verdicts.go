package sched

import (
	"cmp"
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/disk"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// verdictOf is the full explain for one stored ticket.
func (x *Explainer) verdictOf(e tickets.Epic, t tickets.Ticket, addr tickets.Address) wire.Explanation {
	// A launch is tracked before the index may have reread its claim.
	claimed := t.Status == "claimed" || x.Registry.Has(addr.String())
	// A rate-limit hold outranks every other wait reason.
	if t, ok := x.Registry.Tracked(addr.String()); ok && t.RateLimited {
		return wire.Explanation{Address: addr.String(), Verdict: RateLimitVerdict(t.RateLimitReset)}
	}
	// A live agent waiting for its answer outranks the stored status.
	if v, ok := x.waitingVerdict(addr); ok {
		return wire.Explanation{Address: addr.String(), Verdict: v}
	}
	d := ralphloop.TicketVerdict(e, ralphloop.WholeEpicScope(), t, claimed, false)
	switch d.Decision {
	case ralphloop.DecisionStalled:
		return wire.Explanation{Address: addr.String(), Verdict: string(d.Decision), Reason: d.Status} // asking, stuck or draft
	case ralphloop.DecisionUnclaimed:
		if path, missing := x.Projects.UnavailablePath(addr.Project); missing {
			return wire.Explanation{Address: addr.String(), Verdict: wire.VerdictProjectUnavailable, Reason: path + " does not exist"}
		}
		return x.scheduleVerdict(e, t, addr)
	case ralphloop.DecisionBlocked:
		// The scheduler's reason lists only Blocked by: tokens, so a fork child
		// held by its parent alone would explain as blocked by nothing.
		if !e.ParentDone(t) {
			parent := "parent " + string(*t.Parent)
			if d.Reason == "" {
				d.Reason = parent
			} else {
				d.Reason += ", " + parent
			}
		}
	}
	return wire.Explanation{Address: addr.String(), Verdict: string(d.Decision), Reason: d.Reason}
}

// RateLimitVerdict is the whole phrase a held run reads as: the verdict line
// joins verdict and reason with a colon, so the phrase is carried as the verdict.
func RateLimitVerdict(resetAt time.Time) string {
	if resetAt.IsZero() {
		return wire.VerdictRateLimited
	}
	return wire.VerdictRateLimited + ": resets " + resetAt.UTC().Format(time.RFC3339)
}

// waitingVerdict is the verdict of a ticket whose live agent waits for an answer.
func (x *Explainer) waitingVerdict(addr tickets.Address) (string, bool) {
	st, def := x.Settings.Load(), config.DefaultExecutionQueueConfig()
	grace := cmp.Or(st.WaitingSlotGrace, def.WaitingSlotGrace)
	w := x.Registry.WaitStateOf(addr.String(), time.Now(), grace, st.MaxReleasedAgents)
	switch {
	case !w.Waiting:
		return "", false
	case w.Released:
		return wire.VerdictWaitingReleased, true
	case w.CapHeld:
		return wire.VerdictWaitingReleaseCapd, true
	}
	return wire.VerdictWaitingOnAnswer, true
}

// scheduleVerdict explains an unclaimed ticket that nothing in the store blocks:
// where it stands in the queue and the slots.
func (x *Explainer) scheduleVerdict(e tickets.Epic, t tickets.Ticket, addr tickets.Address) wire.Explanation {
	ex := wire.Explanation{Address: addr.String()}
	if x.Guards.Refused.Has(ex.Address) {
		ex.Verdict = wire.VerdictClaimRereadMismatch
		return ex
	}
	root := addr.Project + ":" + addr.Epic
	ahead, queued := x.rootsAhead(root)
	if !queued {
		ex.Verdict = wire.VerdictNotQueued
		return ex
	}
	if reason, missing := x.Rec.TrunkMissing.Get(addr.Project); missing {
		ex.Verdict, ex.Reason = wire.VerdictTrunkMissing, reason
		return ex
	}
	if reason, blocked := x.Rec.MigrationBlock(addr.Project); blocked {
		ex.Verdict, ex.Reason = wire.VerdictMigrationBlocked, reason
		return ex
	}
	if d, held := x.Disks.HeldDiskOf(addr.Project); held {
		ex.Verdict, ex.Reason = wire.VerdictDiskLow, disk.LowReason(d)
		return ex
	}
	if h, held := x.ClaimHold(addr.Project, t, x.Harness.ResolveTicket(addr, t).ResolvedHarness); held {
		ex.Verdict, ex.Reason = h.Verdict, h.Reason
		return ex
	}
	// Every full cap is named, not just the first one hit.
	var full []string
	rootCap := x.Scheduler.PerRootLimit(queue.RootOf(addr))
	if n, limit := x.Registry.CountRoot(root), rootCap; n >= limit {
		full = append(full, fmt.Sprintf("epic cap %d/%d", n, limit))
	}
	if n, limit, atCap := x.Scheduler.ProjectAtCap(addr.Project); atCap {
		full = append(full, fmt.Sprintf("project cap %d/%d", n, limit))
	}
	limit, running := x.Scheduler.ConcurrencyLimit(), x.Registry.Count()
	free := limit - running
	if free <= 0 {
		full = append(full, fmt.Sprintf("global cap %d/%d", running, limit))
	}
	switch {
	case len(full) > 0:
		ex.Verdict, ex.Reason = wire.VerdictConcurrencyCap, strings.Join(full, ", ")
	case frontierIndex(e, t) >= rootCap-x.Registry.CountRoot(root):
		ex.Verdict, ex.Reason = wire.VerdictWaitingInQueue, "earlier tickets of the epic go first"
	case ahead >= free:
		ex.Verdict, ex.Reason = wire.VerdictWaitingInQueue, fmt.Sprintf("%d queued root(s) ahead for %d free slot(s)", ahead, free)
	default:
		ex.Verdict = wire.VerdictEligible
	}
	return ex
}

// frontierIndex is t's place among the epic's claimable tickets, or -1.
func frontierIndex(e tickets.Epic, t tickets.Ticket) int {
	for i, f := range ralphloop.Frontier(e) {
		if f.Identifier == t.Identifier {
			return i
		}
	}
	return -1
}

// rootsAhead counts the distinct idle roots queued before root, and whether
// root is queued at all. Running roots hold a slot already, so they don't count.
func (x *Explainer) rootsAhead(root string) (ahead int, queued bool) {
	seen := map[string]bool{}
	for _, item := range x.Queue.List() {
		a, err := tickets.ParseQueuedAddress(item.Address)
		if err != nil {
			continue
		}
		r := a.Project + ":" + a.Epic
		if r == root {
			return ahead, true
		}
		if !seen[r] && x.Registry.CountRoot(r) == 0 {
			seen[r] = true
			ahead++
		}
	}
	return ahead, false
}
