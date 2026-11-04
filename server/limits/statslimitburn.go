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

func buildLimitBurn(samples []wire.LimitSample, latest wire.LimitSample, hasLatest, weekly bool, n int, now time.Time) wire.StatsLimitBurn {
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

// BurnStats is the burn of each limit over the last n days.
func (l *Limits) BurnStats(n int, now time.Time) wire.StatsLimitBurns {
	build := func(id string, weekly bool) wire.StatsLimitBurn {
		latest, ok := l.history.Latest(id)
		return buildLimitBurn(l.history.Samples(id), latest, ok, weekly, n, now)
	}
	return wire.StatsLimitBurns{
		Claude5h:     build(wire.LimitIDClaude5h, false),
		ClaudeWeekly: build(wire.LimitIDClaudeWeekly, true),
		Codex5h:      build(wire.LimitIDCodex5h, false),
		CodexWeekly:  build(wire.LimitIDCodexWeekly, true),
	}
}
