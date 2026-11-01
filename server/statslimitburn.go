package server

import "time"

// StatsLimitBurn is one limit's burn over the range: one Days point per entry
// of Stats.Days. Sum and Average cover the days with data; StartDay is the first
// day with data and is set only when the range reaches before it. Forecast is
// set on weekly limits that have a reading.
type StatsLimitBurn struct {
	Days     []LimitBurnDay `json:"days"`
	Sum      float64        `json:"sum"`
	Average  float64        `json:"average"`
	StartDay string         `json:"start_day,omitempty"`
	Forecast *LimitForecast `json:"forecast,omitempty"`
}

// StatsLimitBurns is the Limit burn section. It is account-wide: the project
// filter does not change it.
type StatsLimitBurns struct {
	Claude5h     StatsLimitBurn `json:"claude_5h"`
	ClaudeWeekly StatsLimitBurn `json:"claude_weekly"`
	Codex5h      StatsLimitBurn `json:"codex_5h"`
	CodexWeekly  StatsLimitBurn `json:"codex_weekly"`
}

func newStatsLimitBurn(n int) StatsLimitBurn {
	return StatsLimitBurn{Days: make([]LimitBurnDay, n)}
}

func newStatsLimitBurns(n int) StatsLimitBurns {
	return StatsLimitBurns{
		Claude5h: newStatsLimitBurn(n), ClaudeWeekly: newStatsLimitBurn(n),
		Codex5h: newStatsLimitBurn(n), CodexWeekly: newStatsLimitBurn(n),
	}
}

func buildLimitBurn(samples []LimitSample, latest LimitSample, hasLatest, weekly bool, n int, now time.Time) StatsLimitBurn {
	out := StatsLimitBurn{Days: limitBurnDays(samples, n, now)}
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

func (s *Server) buildLimitBurns(n int, now time.Time) StatsLimitBurns {
	build := func(id string, weekly bool) StatsLimitBurn {
		latest, ok := s.limitHistory.Latest(id)
		return buildLimitBurn(s.limitHistory.Samples(id), latest, ok, weekly, n, now)
	}
	return StatsLimitBurns{
		Claude5h:     build(LimitIDClaude5h, false),
		ClaudeWeekly: build(LimitIDClaudeWeekly, true),
		Codex5h:      build(LimitIDCodex5h, false),
		CodexWeekly:  build(LimitIDCodexWeekly, true),
	}
}
