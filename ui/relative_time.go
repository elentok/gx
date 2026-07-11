package ui

import (
	"strconv"
	"time"
)

// RelativeTimeCompact formats recent times for dense TUI tables, for example
// "2h ago", "3d ago", or "1wk 2d ago".
func RelativeTimeCompact(t time.Time) string {
	return relativeTimeCompact(t, true)
}

// RelativeTimeCompactShort is RelativeTimeCompact without the trailing " ago",
// for example "2h", "3d", or "1wk 2d". Used by condensed rows in narrow panels.
func RelativeTimeCompactShort(t time.Time) string {
	return relativeTimeCompact(t, false)
}

func relativeTimeCompact(t time.Time, withAgo bool) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return compactAgo(int(d/time.Minute), "m", withAgo)
	case d < 24*time.Hour:
		return compactAgo(int(d/time.Hour), "h", withAgo)
	case d < 7*24*time.Hour:
		return compactAgo(int(d/(24*time.Hour)), "d", withAgo)
	case d < 30*24*time.Hour:
		weeks := int(d / (7 * 24 * time.Hour))
		days := int((d % (7 * 24 * time.Hour)) / (24 * time.Hour))
		if days > 0 {
			return withAgoSuffix(strconv.Itoa(weeks)+"wk "+strconv.Itoa(days)+"d", withAgo)
		}
		return compactAgo(weeks, "wk", withAgo)
	default:
		months := int(d / (30 * 24 * time.Hour))
		if months < 12 {
			return compactAgo(months, "mo", withAgo)
		}
		years := months / 12
		remainingMonths := months % 12
		if remainingMonths > 0 {
			return withAgoSuffix(strconv.Itoa(years)+"y "+strconv.Itoa(remainingMonths)+"mo", withAgo)
		}
		return compactAgo(years, "y", withAgo)
	}
}

func compactAgo(n int, unit string, withAgo bool) string {
	return withAgoSuffix(strconv.Itoa(n)+unit, withAgo)
}

func withAgoSuffix(s string, withAgo bool) string {
	if withAgo {
		return s + " ago"
	}
	return s
}
