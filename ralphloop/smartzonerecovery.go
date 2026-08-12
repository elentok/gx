package ralphloop

import (
	"errors"
	"fmt"
)

// smartZoneRecovery holds the smart-zone-breach sub-state that waitForFinish's
// main loop carries across poll ticks: whether the just-completed recovery's
// own side effects should be absorbed rather than mistaken for an
// operator-raised block (see consumeJustRecovered), and the gated-give-up
// bookkeeping a breach's recovery can leave behind for a later "finished" poll
// to resolve (see pendingUnresolved). Keeping this behind one type gives the
// main loop a narrow interface — checkAndRecover, consumeJustRecovered,
// pendingUnresolved, recordGatedGiveUp — instead of three loop-local variables
// threaded through every branch.
type smartZoneRecovery struct {
	zone int

	// justRecovered guards the poll tick immediately after checkAndRecover's
	// recoverSmartZoneBreach call: its own ctrl+c-interrupt-and-compact
	// sequence can leave the pane reporting "blocked" as a side effect (a
	// confirmation dialog it didn't fully clear, an unconfirmed compact) that
	// looks identical to an operator-raised prompt. It only ever covers the
	// one immediately-following tick, not the recovery call itself — the
	// recovery call runs synchronously, so there is no poll tick during it to
	// guard.
	justRecovered bool

	// gatedGiveUps counts consecutive recoveries that ended in a gated
	// give-up (see errCompactNeverConfirmed) within this call to waitForFinish.
	gatedGiveUps int

	// pending is the compaction-boundary snapshot taken just before the
	// "/compact" of the most recent gated give-up, kept alive until some later
	// read proves a boundary landed. While it is unresolved the pane's own
	// "finished" report proves nothing: the shape that produces a gated
	// give-up in the first place is a pane reporting idle throughout a
	// running compaction, and that same pane answers the ordinary finish poll
	// idle too.
	pending *compactBoundarySnapshot
}

func newSmartZoneRecovery(zone int) *smartZoneRecovery {
	return &smartZoneRecovery{zone: zone}
}

// consumeJustRecovered reports whether the tick right after a recovery is
// still pending, clearing the flag so only that one tick is guarded.
func (r *smartZoneRecovery) consumeJustRecovered() bool {
	if !r.justRecovered {
		return false
	}
	r.justRecovered = false
	return true
}

// pendingUnresolved reports whether a prior gated give-up's compaction still
// has nothing in the transcript to show for it, i.e. a "finished" poll result
// right now would be that same uncorroborated claim rather than a real finish.
func (r *smartZoneRecovery) pendingUnresolved(d Deps, p launchAndPromptParams, sessionID string) bool {
	return r.pending != nil && r.pending.unresolved(d, p, sessionID)
}

// recordGatedGiveUp counts another consecutive gated give-up and reports
// whether that reaches maxConsecutiveGatedGiveUps, at which point the caller
// must stop retrying and escalate.
func (r *smartZoneRecovery) recordGatedGiveUp() (exceeded bool) {
	r.gatedGiveUps++
	return r.gatedGiveUps >= maxConsecutiveGatedGiveUps
}

// checkAndRecover reads the session's current context occupancy and, on a
// breach, interrupts the pane and runs recoverSmartZoneBreach, updating the
// gated-give-up bookkeeping from its outcome. breached reports whether a
// breach was found at all (whether or not recovery itself succeeded) — the
// caller resets its own elapsed-poll-time counter only when breached is true.
// A non-nil err means the caller must abort waitForFinish entirely, same as
// every other hard failure in the main loop.
func (r *smartZoneRecovery) checkAndRecover(d Deps, p launchAndPromptParams, sessionID string) (breached bool, err error) {
	occupancy, found, decidable, occErr := smartZoneOccupancy(d, p.Agent, p.SessionCwd, sessionID)
	if occErr == nil && found {
		p.sink().ContextOccupancy(p.Ticket, occupancy)
	}
	if occErr != nil || !decidable || occupancy <= r.zone {
		return false, nil
	}

	if err := d.AgentSendKeys(p.Pane, "ctrl+c"); err != nil {
		return false, fmt.Errorf("interrupting %s after smart-zone breach: %w", p.Label, err)
	}
	reason := fmt.Sprintf("context occupancy %d exceeds --smart-zone %d", occupancy, r.zone)
	// Snapshotted here rather than read back out of recoverSmartZoneBreach:
	// the gate's predicate is "count greater than the count at submission
	// time", so the snapshot only means anything if it is taken before
	// "/compact" goes out.
	baseline := readCompactBoundaries(d, p.Agent, p.SessionCwd, sessionID)
	recovered, recErr := recoverSmartZoneBreach(d, p, sessionID, reason, r.zone)
	switch {
	case errors.Is(recErr, errCompactNeverConfirmed):
		r.pending = &baseline
		if r.recordGatedGiveUp() {
			return true, gatedGiveUpsExhausted(p.Label, r.gatedGiveUps, recErr)
		}
	case recErr != nil:
		return true, recErr
	case recovered:
		// Only the bool says a recovery actually completed. The non-gated
		// failure branch returns (false, nil), reporting itself through the
		// sink instead of the error, so resetting on a nil error would clear
		// the counter for precisely the failures that prove nothing about
		// whether compaction is progressing.
		r.gatedGiveUps = 0
		r.pending = nil
	}
	r.justRecovered = true
	return true, nil
}
