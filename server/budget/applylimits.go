package budget

import (
	"time"

	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
)

// ApplyLimits applies each budget limit whose file value changed since
// the last load to the ledger's live limit, then re-evaluates the latches as
// `gx budget increase` does. Comparing file values, not live ones, keeps a
// live increase on the field the person did not edit. A lowered hard limit
// already reached starts the grace countdown, whose start banner is the only
// report of it, so the reload's own banner stays a plain one; a raise that
// lifts the hold cancels it.
func (b *Budget) ApplyLimits(old, next settings.Settings) {
	softChanged := next.BudgetSoftLimit != old.BudgetSoftLimit
	hardChanged := next.BudgetHardLimit != old.BudgetHardLimit
	if !softChanged && !hardChanged {
		return
	}
	now := time.Now()
	soft, hard := b.ledger.limits()
	if softChanged {
		soft = next.BudgetSoftLimit
	}
	if hardChanged {
		held, used := b.ledger.setLimitsHoldingHard(now, soft, next.BudgetHardLimit)
		if held {
			b.startHardGrace(now, next.BudgetHardLimit, used)
		}
	} else {
		b.ledger.setLimits(now, soft, hard)
	}
	if err := b.ledger.save(now); err != nil {
		b.log.Error("save budget ledger", "err", err)
	}
	b.cancelHardGrace()
	b.rearmNotes(b.Status(now))
	b.events.Publish(wire.EventQueueChanged, "")
}
