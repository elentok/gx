package server

import (
	"time"

	"github.com/elentok/gx/server/wire"
)

func newStatsLimitBurn(n int) wire.StatsLimitBurn {
	return wire.StatsLimitBurn{Days: make([]wire.LimitBurnDay, n)}
}

func newStatsLimitBurns(n int) wire.StatsLimitBurns {
	return wire.StatsLimitBurns{
		Claude5h: newStatsLimitBurn(n), ClaudeWeekly: newStatsLimitBurn(n),
		Codex5h: newStatsLimitBurn(n), CodexWeekly: newStatsLimitBurn(n),
	}
}

func buildLimitBurn(samples []LimitSample, latest LimitSample, hasLatest, weekly bool, n int, now time.Time) wire.StatsLimitBurn {
	out := wire.StatsLimitBurn{Days: limitBurnDays(samples, n, now)}
	withData := 0
	for _, d := range out.Days {
		if d.NoData {
			continue
		}
		withData++
		out.Sum += d.Burn
	}
	if withData > 0 {
		out.Average = out.Sum / float64(withData)
		if withData < n {
			out.StartDay = limitDayKey(samples[0].ReceivedAt)
		}
	}
	if weekly && hasLatest {
		f := limitForecast(samples, latest, now)
		out.Forecast = &f
	}
	return out
}

func (s *Server) buildLimitBurns(n int, now time.Time) wire.StatsLimitBurns {
	build := func(id string, weekly bool) wire.StatsLimitBurn {
		latest, ok := s.limitHistory.Latest(id)
		return buildLimitBurn(s.limitHistory.Samples(id), latest, ok, weekly, n, now)
	}
	return wire.StatsLimitBurns{
		Claude5h:     build(LimitIDClaude5h, false),
		ClaudeWeekly: build(LimitIDClaudeWeekly, true),
		Codex5h:      build(LimitIDCodex5h, false),
		CodexWeekly:  build(LimitIDCodexWeekly, true),
	}
}
