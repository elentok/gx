// Package limitseed is the data of gxfixture's limit-burn shapes: the Claude
// limits file, the limit history and the cost ledger. It only builds values;
// gxfixture writes them. It lives apart so gxfixture stays under the package
// size limit.
package limitseed

import (
	"fmt"
	"sort"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tokenledger"
)

// Project is the fixture project the stamped tickets and ledger rows belong to.
const Project = "demo"

// StatsHistoryEpic holds the stamped tickets of the stats shape.
const StatsHistoryEpic = "stats-history"

// Seed is the state of a limit-burn shape: the Claude limits file and the limit
// history, whose file shape comes from server.limitHistoryState.
type Seed struct {
	Report  wire.LimitsReport
	Samples map[string][]wire.LimitSample
	// Ledger is the token ledger and the stamped tickets behind it; nil leaves
	// both empty.
	Ledger *Ledger
}

// Ledger is the cost data of a shape. Rebuilt rows are written first, as the
// backfill would; live rows are written with the clock at LiveSince, so the
// ledger's first live day is that day. Stamped tickets feed the days before the
// first rebuilt day.
type Ledger struct {
	Rebuilt, Live []tokenledger.Row
	LiveSince     time.Time
	Stamped       []Stamped
}

// Stamped is a finished ticket carrying its token total.
type Stamped struct {
	ID, Slug string
	Opts     servertest.TicketOpts
}

// Shapes are the --shape values of the limit-burn scenario. Every time is built
// from the real clock, and the daily steps sit on local midnights, so "today so
// far" and "yesterday" stay right whatever the hour.
var Shapes = map[string]func(now time.Time) Seed{
	// menu: Claude weekly at 81% (7 so far today, 5 yesterday, forecast over
	// 100%), Codex weekly at 40%, both 5-hour rows.
	"menu": menuSeed,
	// live: the menu limits and an empty ledger, with one held agent per cost
	// group (see gxfixture's limitBurnLive).
	"live": menuSeed,
	// few-days: Claude weekly at 22%, the first sample earlier today, no Codex data.
	"few-days": func(now time.Time) Seed {
		reset := LocalMidnight(now).AddDate(0, 0, 4)
		first := LocalMidnight(now).Add(now.Sub(LocalMidnight(now)) / 2)
		return Seed{
			Report: wire.LimitsReport{SevenDay: Window(now, 22, reset.Sub(now), 0)},
			Samples: map[string][]wire.LimitSample{wire.LimitIDClaudeWeekly: {
				sample(wire.LimitIDClaudeWeekly, 15, reset, first),
				sample(wire.LimitIDClaudeWeekly, 22, reset, now),
			}},
		}
	},
	// stale: Claude and Codex weekly with 7 days of history, the reset ahead and
	// the newest reading half an hour old.
	"stale": func(now time.Time) Seed {
		m := LocalMidnight(now)
		claudeReset, codexReset := m.AddDate(0, 0, 2), m.AddDate(0, 0, 3)
		const age = 30 * time.Minute
		return Seed{
			Report: wire.LimitsReport{SevenDay: Window(now, 52, claudeReset.Sub(now), age)},
			Samples: map[string][]wire.LimitSample{
				wire.LimitIDClaudeWeekly: weeklyHistory(wire.LimitIDClaudeWeekly, now, claudeReset, age,
					[]float64{8, 16, 24, 33, 40}, 52, []float64{50, 70}),
				wire.LimitIDCodexWeekly: weeklyHistory(wire.LimitIDCodexWeekly, now, codexReset, age,
					[]float64{5, 10, 17, 25}, 40, []float64{20, 33}),
			},
		}
	},
	// past-reset: Claude weekly whose reset time is two hours gone, with history from three days ago so the row keeps its burn chip.
	"past-reset": func(now time.Time) Seed {
		reset := now.Add(-2 * time.Hour)
		return Seed{
			Report: wire.LimitsReport{SevenDay: Window(now, 75, -2*time.Hour, 6*time.Hour)},
			Samples: map[string][]wire.LimitSample{wire.LimitIDClaudeWeekly: {
				sample(wire.LimitIDClaudeWeekly, 30, reset, now.Add(-72*time.Hour)),
				sample(wire.LimitIDClaudeWeekly, 60, reset, now.Add(-30*time.Hour)),
				sample(wire.LimitIDClaudeWeekly, 75, reset, now.Add(-6*time.Hour)),
			}},
		}
	},
	// stats: 30 days of cost data and 23 days (22 days ago to today) of history
	// for all four limits, so the head of a 30-day range has no limit data. Each
	// history has reset days. The ledger has stamp-only days (29 to 21 days ago,
	// one with an unstamped ticket, so no data), rebuilt days (20 to 11) and live days (10 to today,
	// 5 days ago quiet), across four groups, both harnesses and two projects.
	"stats": func(now time.Time) Seed {
		const historyDays = 22
		weekly := []int{17, 10, 3}
		fiveHour := []int{14, 6, 0}
		s := Seed{
			Samples: map[string][]wire.LimitSample{
				wire.LimitIDClaudeWeekly: burnHistory(wire.LimitIDClaudeWeekly, now, historyDays, 7*24*time.Hour, weekly, 3),
				wire.LimitIDCodexWeekly:  burnHistory(wire.LimitIDCodexWeekly, now, historyDays, 7*24*time.Hour, weekly, 5),
				wire.LimitIDClaude5h:     burnHistory(wire.LimitIDClaude5h, now, historyDays, 5*time.Hour, fiveHour, 2),
				wire.LimitIDCodex5h:      burnHistory(wire.LimitIDCodex5h, now, historyDays, 5*time.Hour, fiveHour, 4),
			},
			Ledger: statsLedger(now),
		}
		last := func(id string) wire.LimitSample { v := s.Samples[id]; return v[len(v)-1] }
		window := func(id string) *wire.LimitWindow {
			l := last(id)
			return &wire.LimitWindow{UsedPercentage: l.UsedPercentage, ResetsAt: l.ResetsAt, ReceivedAt: l.ReceivedAt}
		}
		s.Report = wire.LimitsReport{FiveHour: window(wire.LimitIDClaude5h), SevenDay: window(wire.LimitIDClaudeWeekly)}
		return s
	},
}

// ShapeNames are the sorted keys of Shapes.
func ShapeNames() []string {
	names := make([]string, 0, len(Shapes))
	for n := range Shapes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func menuSeed(now time.Time) Seed {
	m := LocalMidnight(now)
	claudeReset := m.AddDate(0, 0, 2)
	codexReset := m.AddDate(0, 0, 3)
	s := Seed{
		Report: wire.LimitsReport{
			FiveHour: Window(now, 34, 2*time.Hour+10*time.Minute, 0),
			SevenDay: Window(now, 81, claudeReset.Sub(now), 0),
		},
		Samples: map[string][]wire.LimitSample{},
	}
	s.Samples[wire.LimitIDClaudeWeekly] = weeklyHistory(wire.LimitIDClaudeWeekly, now, claudeReset, 0,
		[]float64{20, 36, 52, 69, 74}, 81, []float64{20, 99})
	s.Samples[wire.LimitIDCodexWeekly] = weeklyHistory(wire.LimitIDCodexWeekly, now, codexReset, 0,
		[]float64{14, 20, 24, 36}, 40, []float64{2, 45})
	s.Samples[wire.LimitIDClaude5h] = fiveHourHistory(wire.LimitIDClaude5h, now, 2*time.Hour+10*time.Minute, 10, 34)
	s.Samples[wire.LimitIDCodex5h] = fiveHourHistory(wire.LimitIDCodex5h, now, 3*time.Hour, 5, 12)
	return s
}

// Window resets in resetsIn and was received receivedAgo before now.
func Window(now time.Time, pct float64, resetsIn, receivedAgo time.Duration) *wire.LimitWindow {
	return &wire.LimitWindow{
		UsedPercentage: pct,
		ResetsAt:       now.Add(resetsIn).Unix(),
		ReceivedAt:     now.Add(-receivedAgo).Unix(),
	}
}

// LocalMidnight is the start of t's local day.
func LocalMidnight(t time.Time) time.Time {
	y, mo, d := t.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, t.Location())
}

func sample(id string, pct float64, resetsAt, receivedAt time.Time) wire.LimitSample {
	return wire.LimitSample{LimitID: id, UsedPercentage: pct, ResetsAt: resetsAt.Unix(), ReceivedAt: receivedAt.Unix()}
}

// Harness names in ledger rows.
const (
	ledgerClaude = "claude"
	ledgerCodex  = "codex"
)

// statsLedger builds the cost data of the stats shape (see its comment).
func statsLedger(now time.Time) *Ledger {
	day := func(ago int) string { return LocalMidnight(now).AddDate(0, 0, -ago).Format(tokenledger.DayLayout) }
	other := "docs-site"
	combos := []tokenledger.Key{
		{Project: Project, Group: tokenledger.GroupQueue, Harness: ledgerClaude},
		{Project: Project, Group: tokenledger.GroupDesign, Harness: ledgerClaude},
		{Project: other, Group: tokenledger.GroupQueue, Harness: ledgerCodex},
		{Project: Project, Group: tokenledger.GroupOneOff, Harness: ledgerCodex},
		{Project: other, Group: tokenledger.GroupOther, Harness: ledgerClaude},
	}
	rows := func(from, to int, quiet int) []tokenledger.Row {
		var out []tokenledger.Row
		for ago := from; ago >= to; ago-- {
			if ago == quiet {
				continue
			}
			for i, k := range combos {
				if (ago+i)%2 != 0 {
					continue
				}
				k.Day = day(ago)
				tokens := int64(40_000 + ago*3_100 + i*17_000)
				dollars := float64(tokens) / 80_000
				if k.Harness == ledgerCodex {
					dollars = 0 // Codex reports tokens, not a price.
				}
				out = append(out, tokenledger.Row{Key: k, Tokens: tokens, Dollars: dollars})
			}
		}
		return out
	}
	l := &Ledger{
		Rebuilt:   rows(20, 11, -1),
		Live:      rows(10, 0, 5),
		LiveSince: LocalMidnight(now).AddDate(0, 0, -10).Add(time.Hour),
	}
	for ago := 29; ago >= 21; ago-- {
		tokens := int64(60_000 + ago*2_300)
		if ago == 25 {
			// Finished without a stamp: a no-data day, not a zero one.
			tokens = 0
		}
		l.Stamped = append(l.Stamped, Stamped{fmt.Sprintf("%02d", 30-ago), fmt.Sprintf("stamped-%d-days-ago", ago), servertest.TicketOpts{
			Status:      "done",
			DoneAt:      LocalMidnight(now).AddDate(0, 0, -ago).Add(14 * time.Hour).Format(time.RFC3339),
			TotalTokens: tokens,
			ActualCost:  float64(tokens) / 80_000,
		}})
	}
	return l
}
