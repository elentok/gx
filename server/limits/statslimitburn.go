package limits

import (
	"time"

	"github.com/elentok/gx/server/wire"
)

func newStatsLimitBurn(n int) wire.StatsLimitBurn {
	return wire.StatsLimitBurn{Days: make([]wire.LimitBurnDay, n)}
}

// NewStatsLimitBurns is the all-zero burn of every limit over n days.
func NewStatsLimitBurns(n int) wire.StatsLimitBurns {
	return wire.StatsLimitBurns{
		Claude5h: newStatsLimitBurn(n), ClaudeWeekly: newStatsLimitBurn(n),
		Codex5h: newStatsLimitBurn(n), CodexWeekly: newStatsLimitBurn(n),
	}
}

// buildLimitBurn adds a forecast from forecastFrom when it is non-nil.
func buildLimitBurn(samples []wire.LimitSample, forecastFrom *wire.LimitSample, n int, now time.Time) wire.StatsLimitBurn {
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
	if forecastFrom != nil {
		f := limitForecast(samples, *forecastFrom, now)
		out.Forecast = &f
	}
	return out
}

// BurnStats is the burn of each limit over the last n days.
func (l *Limits) BurnStats(n int, now time.Time) wire.StatsLimitBurns {
	build := func(id string, forecastFrom *wire.LimitSample) wire.StatsLimitBurn {
		return buildLimitBurn(l.history.Samples(id), forecastFrom, n, now)
	}
	// The menu forecasts Claude weekly from the live window when no sample is
	// saved; Stats does the same. Codex has no live window, so it needs a sample.
	var claudeWeekly, codexWeekly *wire.LimitSample
	if s, ok := l.history.Latest(wire.LimitIDClaudeWeekly); ok {
		claudeWeekly = &s
	} else if live := l.store.read().SevenDay; live != nil {
		s := l.latestOr(wire.LimitIDClaudeWeekly, *live)
		claudeWeekly = &s
	}
	if s, ok := l.history.Latest(wire.LimitIDCodexWeekly); ok {
		codexWeekly = &s
	}
	return wire.StatsLimitBurns{
		Claude5h:     build(wire.LimitIDClaude5h, nil),
		ClaudeWeekly: build(wire.LimitIDClaudeWeekly, claudeWeekly),
		Codex5h:      build(wire.LimitIDCodex5h, nil),
		CodexWeekly:  build(wire.LimitIDCodexWeekly, codexWeekly),
	}
}
