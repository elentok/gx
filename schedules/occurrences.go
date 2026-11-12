package schedules

import (
	"slices"
	"time"
)

// maxScanDays bounds every search: a Schedule with any day set has a set time
// in every 7 day span, so 9 days covers the span plus daylight saving shifts.
const maxScanDays = 9

// Next returns the first set time strictly after `after`, in after's Location.
// It returns the zero time when the Schedule has no set times.
func Next(s Schedule, after time.Time) time.Time {
	loc := after.Location()
	y, m, d := after.Date()
	for off := -1; off <= maxScanDays; off++ {
		var best time.Time
		for _, t := range dayTimes(s, time.Date(y, m, d+off, 0, 0, 0, 0, loc)) {
			if t.After(after) && (best.IsZero() || t.Before(best)) {
				best = t
			}
		}
		if !best.IsZero() {
			return best
		}
	}
	return time.Time{}
}

// LatestDue returns the latest set time t with after < t <= now, in now's
// Location, or the zero time when none is due. Older missed times are ignored.
func LatestDue(s Schedule, after, now time.Time) time.Time {
	loc := now.Location()
	y, m, d := now.Date()
	for off := 0; off >= -maxScanDays; off-- {
		day := time.Date(y, m, d+off, 0, 0, 0, 0, loc)
		var best time.Time
		for _, t := range dayTimes(s, day) {
			if t.After(after) && !t.After(now) && t.After(best) {
				best = t
			}
		}
		if !best.IsZero() {
			return best
		}
		if day.Add(24 * time.Hour).Before(after) {
			break
		}
	}
	return time.Time{}
}

// dayTimes resolves the Schedule's times on the nominal date of `day` to
// instants, or returns none when the date's weekday is not set. Times that
// collapse to one instant after a gap appear once.
func dayTimes(s Schedule, day time.Time) []time.Time {
	if !slices.Contains(s.Days, day.Weekday()) {
		return nil
	}
	y, m, d := day.Date()
	var out []time.Time
	for _, tm := range s.Times {
		hh, mm, ok := parseHHMM(tm)
		if !ok {
			continue
		}
		t := resolveWall(y, m, d, hh, mm, day.Location())
		if !slices.ContainsFunc(out, t.Equal) {
			out = append(out, t)
		}
	}
	return out
}

// resolveWall maps a wall-clock time to an instant. A time inside a
// spring-forward gap runs at the first moment after the gap; a time that
// occurs twice runs at its first occurrence.
func resolveWall(y int, m time.Month, d, hh, mm int, loc *time.Location) time.Time {
	wall := time.Date(y, m, d, hh, mm, 0, 0, time.UTC)
	_, offBefore := wall.Add(-24 * time.Hour).In(loc).Zone()
	_, offAfter := wall.Add(24 * time.Hour).In(loc).Zone()

	var valid []time.Time
	for _, off := range []int{offBefore, offAfter} {
		c := wall.Add(-time.Duration(off) * time.Second).In(loc)
		if cy, cm, cd := c.Date(); cy == y && cm == m && cd == d && c.Hour() == hh && c.Minute() == mm {
			valid = append(valid, c)
		}
	}
	if len(valid) > 0 {
		return slices.MinFunc(valid, time.Time.Compare)
	}
	// Gap: the later candidate lies after the transition, whose instant is the
	// start of the zone that candidate is in.
	later := wall.Add(-time.Duration(min(offBefore, offAfter)) * time.Second).In(loc)
	start, _ := later.ZoneBounds()
	if start.IsZero() {
		return later
	}
	return start
}

func parseHHMM(s string) (hh, mm int, ok bool) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}
