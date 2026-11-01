package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/elentok/gx/server"
)

// limitBurnSeed is the state of a limit-burn shape: the Claude limits file and
// the limit history, whose file shape comes from server.limitHistoryState.
type limitBurnSeed struct {
	report  server.LimitsReport
	samples map[string][]server.LimitSample
}

// limitBurnShapes are the --shape values of the limit-burn scenario. Every time
// is built from the real clock, and the daily steps sit on local midnights, so
// "today so far" and "yesterday" stay right whatever the hour.
var limitBurnShapes = map[string]func(now time.Time) limitBurnSeed{
	// menu: Claude weekly at 81% (7 so far today, 5 yesterday, forecast over
	// 100%), Codex weekly at 40%, both 5-hour rows.
	"menu": func(now time.Time) limitBurnSeed {
		m := localMidnight(now)
		claudeReset := m.AddDate(0, 0, 2)
		codexReset := m.AddDate(0, 0, 3)
		s := limitBurnSeed{
			report: server.LimitsReport{
				FiveHour: limitWindow(now, 34, 2*time.Hour+10*time.Minute, 0),
				SevenDay: limitWindow(now, 81, claudeReset.Sub(now), 0),
			},
			samples: map[string][]server.LimitSample{},
		}
		s.samples[server.LimitIDClaudeWeekly] = weeklyHistory(server.LimitIDClaudeWeekly, now, claudeReset, 0,
			[]float64{20, 36, 52, 69, 74}, 81, []float64{60, 88})
		s.samples[server.LimitIDCodexWeekly] = weeklyHistory(server.LimitIDCodexWeekly, now, codexReset, 0,
			[]float64{5, 10, 17, 25}, 40, []float64{20, 33})
		s.samples[server.LimitIDClaude5h] = fiveHourHistory(server.LimitIDClaude5h, now, 2*time.Hour+10*time.Minute, 10, 34)
		s.samples[server.LimitIDCodex5h] = fiveHourHistory(server.LimitIDCodex5h, now, 3*time.Hour, 5, 12)
		return s
	},
	// few-days: Claude weekly at 22%, the first sample earlier today, no Codex data.
	"few-days": func(now time.Time) limitBurnSeed {
		reset := localMidnight(now).AddDate(0, 0, 4)
		first := localMidnight(now).Add(now.Sub(localMidnight(now)) / 2)
		return limitBurnSeed{
			report: server.LimitsReport{SevenDay: limitWindow(now, 22, reset.Sub(now), 0)},
			samples: map[string][]server.LimitSample{server.LimitIDClaudeWeekly: {
				limitSample(server.LimitIDClaudeWeekly, 15, reset, first),
				limitSample(server.LimitIDClaudeWeekly, 22, reset, now),
			}},
		}
	},
	// stale: Claude and Codex weekly with 7 days of history, the reset ahead and
	// the newest reading half an hour old.
	"stale": func(now time.Time) limitBurnSeed {
		m := localMidnight(now)
		claudeReset, codexReset := m.AddDate(0, 0, 2), m.AddDate(0, 0, 3)
		const age = 30 * time.Minute
		return limitBurnSeed{
			report: server.LimitsReport{SevenDay: limitWindow(now, 52, claudeReset.Sub(now), age)},
			samples: map[string][]server.LimitSample{
				server.LimitIDClaudeWeekly: weeklyHistory(server.LimitIDClaudeWeekly, now, claudeReset, age,
					[]float64{8, 16, 24, 33, 40}, 52, []float64{50, 70}),
				server.LimitIDCodexWeekly: weeklyHistory(server.LimitIDCodexWeekly, now, codexReset, age,
					[]float64{5, 10, 17, 25}, 40, []float64{20, 33}),
			},
		}
	},
	// past-reset: Claude weekly whose reset time is two hours gone.
	"past-reset": func(now time.Time) limitBurnSeed {
		reset := now.Add(-2 * time.Hour)
		return limitBurnSeed{
			report: server.LimitsReport{SevenDay: limitWindow(now, 75, -2*time.Hour, 6*time.Hour)},
			samples: map[string][]server.LimitSample{server.LimitIDClaudeWeekly: {
				limitSample(server.LimitIDClaudeWeekly, 60, reset, now.Add(-30*time.Hour)),
				limitSample(server.LimitIDClaudeWeekly, 75, reset, now.Add(-6*time.Hour)),
			}},
		}
	},
}

func limitBurnShapeNames() []string {
	names := make([]string, 0, len(limitBurnShapes))
	for n := range limitBurnShapes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func localMidnight(t time.Time) time.Time {
	y, mo, d := t.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, t.Location())
}

func limitSample(id string, pct float64, resetsAt, receivedAt time.Time) server.LimitSample {
	return server.LimitSample{LimitID: id, UsedPercentage: pct, ResetsAt: resetsAt.Unix(), ReceivedAt: receivedAt.Unix()}
}

// weeklyHistory is the history of one weekly window that resets at reset. The
// window opened 7 days before, so dayEnds (one per local day since then,
// earliest first) fill it up to yesterday and today ends at current. Each day
// has a sample just after midnight carrying the day before and one at the end
// of the day. prev is the end-of-day percentages of the window before it, which
// give the 7th day of history. The newest sample was received newestAgo ago.
func weeklyHistory(id string, now, reset time.Time, newestAgo time.Duration, dayEnds []float64, current float64, prev []float64) []server.LimitSample {
	start := reset.AddDate(0, 0, -7)
	var out []server.LimitSample
	for i, pct := range prev {
		day := start.AddDate(0, 0, i-len(prev))
		out = append(out, limitSample(id, pct, start, day.Add(30*time.Minute)))
	}
	carry := 0.0
	for i, pct := range dayEnds {
		day := start.AddDate(0, 0, i)
		out = append(out,
			limitSample(id, carry, reset, day.Add(30*time.Minute)),
			limitSample(id, pct, reset, day.Add(23*time.Hour+30*time.Minute)))
		carry = pct
	}
	newest := now.Add(-newestAgo)
	today := localMidnight(now).Add(30 * time.Minute)
	if today.After(newest.Add(-time.Minute)) {
		today = newest.Add(-time.Minute)
	}
	return append(out, limitSample(id, carry, reset, today), limitSample(id, current, reset, newest))
}

// fiveHourHistory is two samples of a 5-hour window that resets in resetsIn.
func fiveHourHistory(id string, now time.Time, resetsIn time.Duration, before, current float64) []server.LimitSample {
	reset := now.Add(resetsIn)
	return []server.LimitSample{
		limitSample(id, before, reset, now.Add(-time.Hour)),
		limitSample(id, current, reset, now),
	}
}

// seedLimitBurn writes the Claude limits file and the limit history the server
// loads at start. The history file is written as server.limitHistoryState:
// every sample per limit, and the newest one as latest.
func (f *fixture) seedLimitBurn(shape string) error {
	seed := limitBurnShapes[shape](time.Now())
	latest := map[string]server.LimitSample{}
	for id, s := range seed.samples {
		latest[id] = s[len(s)-1]
	}
	if err := os.MkdirAll(f.stateD, 0o700); err != nil {
		return err
	}
	for name, v := range map[string]any{
		"claude-limits.json": seed.report,
		"limit-history.json": map[string]any{"samples": seed.samples, "latest": latest},
	} {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(f.stateD, name), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}
