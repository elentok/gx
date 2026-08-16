package tickets

// budgetLimitLatch is the shared trip/override/re-arm state machine behind
// both checkBudgetSoftLimit and checkBudgetHardLimit (ticket 06/08 latched
// this identically twice under different field names; this is the one
// implementation both now share). A latch's zero value is unarmed —
// costAggregator embeds one per limit and locks around every access with its
// own mu, so the latch itself does no locking.
type budgetLimitLatch struct {
	tripped       bool
	overridden    bool
	overridePoint float64
}

// checkAndTrip runs one tick's worth of latch logic against total/limit and
// reports whether this call latched a fresh trip. The caller must run its
// trip side effects (pause + notify, plus the hard-limit kill) itself, after
// unlocking — never from inside checkAndTrip — since those side effects can
// block for seconds (see costAggregator.checkBudgetHardLimit's kill call)
// and must not hold up unrelated readers like LiveSpend.
//
// Once tripped, a later call with total back under limit never self-clears
// it — only an accepted override, followed by total climbing back past the
// re-arm point computed from overridePoint, re-arms the latch for a fresh
// trip.
func (l *budgetLimitLatch) checkAndTrip(total, limit float64, thresholds []float64) (firedNow bool) {
	if l.overridden {
		rearm := budgetLimitRearmPoint(l.overridePoint, limit, thresholds)
		if total <= rearm {
			return false // still suppressed
		}
		l.overridden = false
		l.tripped = false // re-arm: allow a fresh trip below
	}

	if l.tripped || total < limit {
		return false
	}

	l.tripped = true
	return true
}

// override accepts the operator's confirm-dialog override of an active trip.
func (l *budgetLimitLatch) override(total float64) {
	l.overridden = true
	l.overridePoint = total
}

// reset clears the latch back to its zero value, for reattach (see
// costAggregator.start/stop).
func (l *budgetLimitLatch) reset() {
	*l = budgetLimitLatch{}
}

// budgetLimitRearmPoint implements the re-arm formula: min(next configured
// notification threshold strictly above overridePoint, overridePoint + 10%
// of limit) — well-defined even with no threshold above overridePoint
// (falls back to the +10% arm) or an empty thresholds list. limit is
// whichever of soft/hard tripped this latch — not necessarily the soft
// limit, despite the formula's shared shape.
func budgetLimitRearmPoint(overridePoint, limit float64, thresholds []float64) float64 {
	rearm := overridePoint + 0.1*limit
	for _, th := range thresholds {
		if th > overridePoint && th < rearm {
			rearm = th
		}
	}
	return rearm
}
