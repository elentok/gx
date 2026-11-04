package server

import (
	"reflect"
	"time"

	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
)

// reloadAppliers is the root's registration. The order is the apply order:
// budget, chat, store commits, swap, then certs, config watch, log prune and
// the runner kick.
func (s *Server) reloadAppliers() settings.Appliers {
	return settings.Appliers{
		Before: []settings.Applier{
			func(old, next Settings) func() { s.applyBudgetLimits(old, next); return nil },
			func(old, next Settings) func() {
				if next.Chat == old.Chat {
					return nil
				}
				return s.chat.SetGlobal(next.Chat)
			},
			s.settings.ApplyStoreCommits,
		},
		After: []settings.Applier{
			func(old, next Settings) func() {
				if s.webCerts != nil && !reflect.DeepEqual(old.WebAllowedHosts, next.WebAllowedHosts) {
					if err := s.webCerts.Reissue(); err != nil {
						s.log.Error("web certificate re-issue failed", "err", err)
					}
				}
				return nil
			},
			s.settings.ApplyWatchConfig,
			func(old, next Settings) func() {
				if next.LogRetention != old.LogRetention {
					s.pruneAgentLogs(time.Now())
				}
				return nil
			},
			func(Settings, Settings) func() { s.guards.Kick(); return nil },
		},
	}
}

// banner shows one config-style OS banner and streams it as an event.
func (s *Server) banner(level, title, body string) { s.settings.Banner(level, title, body) }

// applyBudgetLimits applies each budget limit whose file value changed since
// the last load to the ledger's live limit, then re-evaluates the latches as
// `gx budget increase` does. Comparing file values, not live ones, keeps a
// live increase on the field the person did not edit. A lowered hard limit
// already reached starts the grace countdown, whose start banner is the only
// report of it, so the reload's own banner stays a plain one; a raise that
// lifts the hold cancels it.
func (s *Server) applyBudgetLimits(old, next Settings) {
	softChanged := next.BudgetSoftLimit != old.BudgetSoftLimit
	hardChanged := next.BudgetHardLimit != old.BudgetHardLimit
	if !softChanged && !hardChanged {
		return
	}
	now := time.Now()
	soft, hard := s.ledger.limits()
	if softChanged {
		soft = next.BudgetSoftLimit
	}
	if hardChanged {
		held, spent := s.ledger.setLimitsHoldingHard(now, soft, next.BudgetHardLimit)
		if held {
			s.startHardGrace(now, next.BudgetHardLimit, spent)
		}
	} else {
		s.ledger.setLimits(now, soft, hard)
	}
	if err := s.ledger.save(now); err != nil {
		s.log.Error("save budget ledger", "err", err)
	}
	s.cancelHardGrace()
	s.rearmBudgetNotes(s.budgetStatus(now))
	s.events.Publish(wire.EventQueueChanged, "")
}
