package rest

import (
	"time"

	"github.com/elentok/gx/config"
)

// clockMinutes is HH:MM as minutes after midnight. The config guarantees the
// form, so a bad value reads as midnight.
func clockMinutes(hhmm string) int {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return 0
	}
	return t.Hour()*60 + t.Minute()
}

// inBedtime reports whether now falls inside the range. An End earlier than
// Start crosses midnight; End itself is outside.
func inBedtime(b config.MentalHealthBedtime, now time.Time) bool {
	m := now.Hour()*60 + now.Minute()
	start, end := clockMinutes(b.Start), clockMinutes(b.End)
	if start < end {
		return m >= start && m < end
	}
	return m >= start || m < end
}

// nextClock is the first instant after t whose wall clock reads hhmm, in t's
// location.
func nextClock(t time.Time, hhmm string) time.Time {
	at := func(day time.Time) time.Time {
		m := clockMinutes(hhmm)
		return time.Date(day.Year(), day.Month(), day.Day(), m/60, m%60, 0, 0, day.Location())
	}
	c := at(t)
	if !c.After(t) {
		c = at(t.AddDate(0, 0, 1))
	}
	return c
}
