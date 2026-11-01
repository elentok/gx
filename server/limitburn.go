package server

import (
	"math"
	"time"
)

// limitForecastMinDays is the full local days of history a forecast needs, and
// limitPaceDays caps the days the pace averages over.
const (
	limitForecastMinDays = 3
	limitPaceDays        = 7
)

// LimitBurnDay is one local day of a limit's burn. NoData marks a day before the
// limit's first sample; ResetDay marks a day a new window started.
type LimitBurnDay struct {
	Date     string  `json:"date"`
	Burn     float64 `json:"burn"`
	NoData   bool    `json:"no_data"`
	ResetDay bool    `json:"reset_day"`
}

// LimitChipDirection is how today so far compares with yesterday.
type LimitChipDirection string

const (
	LimitChipUp    LimitChipDirection = "up"
	LimitChipDown  LimitChipDirection = "down"
	LimitChipEqual LimitChipDirection = "equal"
)

// LimitBurnChip compares today's burn so far with yesterday's full day, in whole
// points.
type LimitBurnChip struct {
	Today     int                `json:"today"`
	Yesterday int                `json:"yesterday"`
	Direction LimitChipDirection `json:"direction"`
}

// LimitForecastStatus says which forecast line a row shows.
type LimitForecastStatus string

const (
	// LimitForecastNone shows no forecast line.
	LimitForecastNone LimitForecastStatus = "none"
	// LimitForecastNeedsData shows "Forecast needs 3 days of data".
	LimitForecastNeedsData LimitForecastStatus = "needs_data"
	// LimitForecastProjected shows the percentage at reset, and when the limit
	// runs out if RunsOutAt is set.
	LimitForecastProjected LimitForecastStatus = "projected"
)

// LimitForecast is the forecast line of a weekly limit. Pace is points per day.
// AtReset is rounded to whole points; RunsOutAt is epoch seconds and set only
// when the forecast passes 100.
type LimitForecast struct {
	Status    LimitForecastStatus `json:"status"`
	Pace      float64             `json:"pace"`
	AtReset   int                 `json:"at_reset"`
	RunsOutAt int64               `json:"runs_out_at,omitempty"`
}

// limitBurnSeries is the burn of every local day seen in a limit's samples.
type limitBurnSeries struct {
	burn   map[string]float64
	resets map[string]bool
	first  time.Time // receive time of the first sample
	ok     bool
}

func limitDayKey(epoch int64) string {
	return time.Unix(epoch, 0).In(time.Local).Format(statsDayLayout)
}

// newLimitBurnSeries walks samples (oldest first). A rise inside a window is
// credited to the later sample's day. A fall keeps the baseline, so a stale
// reading followed by the real one is not counted twice. A reset time moving
// forward starts a window whose first sample counts in full; one moving backward
// is dropped. The very first sample adds nothing: its percentage was used before
// gx saw it.
func newLimitBurnSeries(samples []LimitSample) limitBurnSeries {
	s := limitBurnSeries{burn: map[string]float64{}, resets: map[string]bool{}}
	var prev LimitSample
	for _, cur := range samples {
		if !s.ok {
			s.ok = true
			s.first = time.Unix(cur.ReceivedAt, 0)
			prev = cur
			continue
		}
		day := limitDayKey(cur.ReceivedAt)
		switch {
		case cur.ResetsAt < prev.ResetsAt:
		case cur.ResetsAt > prev.ResetsAt:
			s.burn[day] += cur.UsedPercentage
			s.resets[day] = true
			prev = cur
		case cur.UsedPercentage > prev.UsedPercentage:
			s.burn[day] += cur.UsedPercentage - prev.UsedPercentage
			prev = cur
		}
	}
	return s
}

func limitDayStart(now time.Time, daysAgo int) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day()-daysAgo, 0, 0, 0, 0, time.Local)
}

// fullDay reports whether the local day daysAgo starts on or after the first
// sample, so that all of it could have been seen.
func (s limitBurnSeries) fullDay(now time.Time, daysAgo int) bool {
	return s.ok && !limitDayStart(now, daysAgo).Before(s.first)
}

func (s limitBurnSeries) dayBurn(now time.Time, daysAgo int) float64 {
	return s.burn[limitDayStart(now, daysAgo).Format(statsDayLayout)]
}

// fullDays is the number of full local days before today, capped at limitPaceDays.
func (s limitBurnSeries) fullDays(now time.Time) int {
	n := 0
	for n < limitPaceDays && s.fullDay(now, n+1) {
		n++
	}
	return n
}

// limitBurnDays is the burn of each of the last n local days, ending today.
func limitBurnDays(samples []LimitSample, n int, now time.Time) []LimitBurnDay {
	s := newLimitBurnSeries(samples)
	win := newStatsWindow(n, now)
	out := make([]LimitBurnDay, n)
	for i, day := range win.days {
		key := day.Format(statsDayLayout)
		out[i] = LimitBurnDay{
			Date:     key,
			Burn:     s.burn[key],
			NoData:   !s.ok || key < s.first.In(time.Local).Format(statsDayLayout),
			ResetDay: s.resets[key],
		}
	}
	return out
}

// limitBurnChip compares today so far with yesterday. It is nil until yesterday
// is a full local day on or after the first sample; a quiet day counts as 0.
func limitBurnChip(samples []LimitSample, now time.Time) *LimitBurnChip {
	s := newLimitBurnSeries(samples)
	if !s.fullDay(now, 1) {
		return nil
	}
	chip := &LimitBurnChip{
		Today:     int(math.Round(s.dayBurn(now, 0))),
		Yesterday: int(math.Round(s.dayBurn(now, 1))),
	}
	switch {
	case chip.Today > chip.Yesterday:
		chip.Direction = LimitChipUp
	case chip.Today < chip.Yesterday:
		chip.Direction = LimitChipDown
	default:
		chip.Direction = LimitChipEqual
	}
	return chip
}

// limitPace is the average burn per day over the last full local days (today
// excluded), and how many days that is.
func limitPace(samples []LimitSample, now time.Time) (pace float64, days int) {
	s := newLimitBurnSeries(samples)
	days = s.fullDays(now)
	if days == 0 {
		return 0, 0
	}
	sum := 0.0
	for i := 1; i <= days; i++ {
		sum += s.dayBurn(now, i)
	}
	return sum / float64(days), days
}

// limitForecast is the forecast line of a weekly limit; latest is the newest
// reading, which may be newer than the last sample. Precedence: a past-reset
// row, then too little history, then a full bar, then a zero pace.
func limitForecast(samples []LimitSample, latest LimitSample, now time.Time) LimitForecast {
	pace, days := limitPace(samples, now)
	none := LimitForecast{Status: LimitForecastNone, Pace: pace}
	if now.Unix() >= latest.ResetsAt {
		return none
	}
	if days < limitForecastMinDays {
		none.Status = LimitForecastNeedsData
		return none
	}
	used := latest.UsedPercentage
	if used >= 100 {
		return none
	}
	f := LimitForecast{Status: LimitForecastProjected, Pace: pace, AtReset: int(math.Round(used))}
	if pace == 0 {
		return f
	}
	daysLeft := float64(latest.ResetsAt-now.Unix()) / 86400
	atReset := used + pace*daysLeft
	f.AtReset = int(math.Round(atReset))
	if atReset > 100 {
		f.RunsOutAt = now.Unix() + int64(math.Round((100-used)/pace*86400))
	}
	return f
}
