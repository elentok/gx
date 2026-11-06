package limitseed

import (
	"slices"
	"time"

	"github.com/elentok/gx/server/wire"
)

// burnHistory is one sample a day from daysAgo days ago to today, each at local
// noon (today's just before now). The limit rises by a few points a day, with
// quiet days, and starts a new window on each reset day: the reset time moves
// forward and the percentage starts again. period is the window length; the last
// window ends after now. step shifts the daily rise per limit.
func burnHistory(id string, now time.Time, daysAgo int, period time.Duration, resetDays []int, step int) []wire.LimitSample {
	var out []wire.LimitSample
	used := float64(step * 3)
	var resetsAt time.Time
	for ago := daysAgo; ago >= 0; ago-- {
		at := LocalMidnight(now).AddDate(0, 0, -ago).Add(12 * time.Hour)
		if ago == 0 && at.After(now) {
			at = now.Add(-time.Minute)
		}
		rise := float64((ago*7+step)%5 + 1)
		if ago%6 == 4 {
			rise = 0
		}
		switch {
		case slices.Contains(resetDays, ago):
			used = rise + 1
			resetsAt = at.Add(period)
			if ago == 0 {
				resetsAt = now.Add(period / 3)
			}
		case resetsAt.IsZero():
			resetsAt = at.Add(period / 2)
			used += rise
		default:
			used += rise
		}
		out = append(out, sample(id, used, resetsAt, at))
	}
	return out
}

// weeklyHistory is the history of one weekly window that resets at reset. The
// window opened 7 days before, so dayEnds (one per local day since then,
// earliest first) fill it up to yesterday and today ends at current. Each day
// has a sample just after midnight carrying the day before and one at the end
// of the day. prev is the end-of-day percentages of the window before it, which
// give the 7th day of history. The newest sample was received newestAgo ago.
func weeklyHistory(id string, now, reset time.Time, newestAgo time.Duration, dayEnds []float64, current float64, prev []float64) []wire.LimitSample {
	start := reset.AddDate(0, 0, -7)
	var out []wire.LimitSample
	for i, pct := range prev {
		day := start.AddDate(0, 0, i-len(prev))
		out = append(out, sample(id, pct, start, day.Add(30*time.Minute)))
	}
	carry := 0.0
	for i, pct := range dayEnds {
		day := start.AddDate(0, 0, i)
		out = append(out,
			sample(id, carry, reset, day.Add(30*time.Minute)),
			sample(id, pct, reset, day.Add(23*time.Hour+30*time.Minute)))
		carry = pct
	}
	newest := now.Add(-newestAgo)
	today := LocalMidnight(now).Add(30 * time.Minute)
	if today.After(newest.Add(-time.Minute)) {
		today = newest.Add(-time.Minute)
	}
	return append(out, sample(id, carry, reset, today), sample(id, current, reset, newest))
}

// fiveHourHistory is two samples of a 5-hour window that resets in resetsIn.
func fiveHourHistory(id string, now time.Time, resetsIn time.Duration, before, current float64) []wire.LimitSample {
	reset := now.Add(resetsIn)
	return []wire.LimitSample{
		sample(id, before, reset, now.Add(-time.Hour)),
		sample(id, current, reset, now),
	}
}
