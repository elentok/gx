package server

import (
	"net/http"
	"time"

	"github.com/elentok/gx/server/wire"
)

// usageLimitWindow builds a row from a window and, for a weekly limit, the
// history of limit id.
func (s *Server) usageLimitWindow(w wire.LimitWindow, id string, weekly bool, now time.Time) *wire.UsageLimitWindow {
	row := &wire.UsageLimitWindow{LimitWindow: w}
	if !weekly {
		return row
	}
	samples := s.limitHistory.Samples(id)
	row.Chip = limitBurnChip(samples, now)
	latest, ok := s.limitHistory.Latest(id)
	if !ok {
		latest = LimitSample{UsedPercentage: w.UsedPercentage, ResetsAt: w.ResetsAt, ReceivedAt: w.ReceivedAt}
	}
	f := limitForecast(samples, latest, now)
	row.Forecast = &f
	return row
}

func (s *Server) codexSummary(now time.Time) *wire.UsageLimits {
	var out wire.UsageLimits
	for _, c := range []struct {
		id     string
		weekly bool
		dst    **wire.UsageLimitWindow
	}{
		{LimitIDCodex5h, false, &out.FiveHour},
		{LimitIDCodexWeekly, true, &out.SevenDay},
	} {
		latest, ok := s.limitHistory.Latest(c.id)
		if !ok {
			continue
		}
		w := wire.LimitWindow{UsedPercentage: latest.UsedPercentage, ResetsAt: latest.ResetsAt, ReceivedAt: latest.ReceivedAt}
		*c.dst = s.usageLimitWindow(w, c.id, c.weekly, now)
	}
	if out.FiveHour == nil && out.SevenDay == nil {
		return nil
	}
	return &out
}

func (s *Server) usageSummaryRead(w http.ResponseWriter, _ *http.Request) {
	limits := s.limits.read()
	mode := wire.UsageModeSpending
	if limits.FiveHour != nil || limits.SevenDay != nil {
		mode = wire.UsageModeLimits
	}
	now := time.Now()
	var claude wire.UsageLimits
	if limits.FiveHour != nil {
		claude.FiveHour = s.usageLimitWindow(*limits.FiveHour, LimitIDClaude5h, false, now)
	}
	if limits.SevenDay != nil {
		claude.SevenDay = s.usageLimitWindow(*limits.SevenDay, LimitIDClaudeWeekly, true, now)
	}
	sw := computeSpendWindows(now, s.ledger.daysCopy())
	writeJSON(w, wire.UsageSummary{
		Mode:   mode,
		Limits: claude,
		Codex:  s.codexSummary(now),
		Spend: wire.UsageSpend{
			Today:            sw.Today.Total,
			SinceSunday:      sw.Week.Total,
			SinceSundayShort: sw.Week.Short,
			SinceFirst:       sw.Month.Total,
			SinceFirstShort:  sw.Month.Short,
			FirstDay:         sw.First,
		},
	})
}
