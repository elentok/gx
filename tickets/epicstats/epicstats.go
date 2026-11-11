// Package epicstats sums an epic's tickets' stats stamps. It sits outside
// tickets so that package stays under its size limits.
package epicstats

import (
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Stats is the sum of an epic's done tickets' stats stamps, plus the
// friction of its cancelled tickets.
type Stats struct {
	// TotalTokens is nil unless every done ticket is stamped, so an epic never
	// shows an undercount; ActualCost is only meaningful with it.
	TotalTokens *int64
	ActualCost  float64
	HadCodex    bool
	Friction    schema.FrictionSummary
}

// Sum adds up the stamps of the done tickets in ts. Friction also
// counts cancelled tickets, so the epic agrees with the Stats page.
func Sum(ts []tickets.Ticket) Stats {
	var stats Stats
	var tokens int64
	var frictions []schema.FrictionSummary
	done, allStamped := 0, true
	for _, t := range ts {
		if t.IsCancelled() && t.Friction != nil {
			frictions = append(frictions, *t.Friction)
		}
		if !t.IsDone() {
			continue
		}
		done++
		stats.HadCodex = stats.HadCodex || t.HadCodex
		if t.Friction != nil {
			frictions = append(frictions, *t.Friction)
		}
		if t.TotalTokens == nil {
			allStamped = false
			continue
		}
		tokens += *t.TotalTokens
		stats.ActualCost += t.ActualCost
	}
	if done > 0 && allStamped {
		stats.TotalTokens = &tokens
	} else {
		stats.ActualCost = 0
	}
	stats.Friction = schema.SumFriction(frictions...)
	return stats
}

// Fields is the stamp for the epic's ticket.md: tokens and cost only when
// TotalTokens is set, the Codex marker only when true.
func (s Stats) Fields() map[string]any {
	fields := map[string]any{"friction": s.Friction}
	if s.TotalTokens != nil {
		fields["total_tokens"] = *s.TotalTokens
		fields["actual_cost"] = s.ActualCost
	}
	if s.HadCodex {
		fields["had_codex"] = true
	}
	return fields
}
