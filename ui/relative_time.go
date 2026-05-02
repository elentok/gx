package ui

import (
	"strconv"
	"time"
)

// RelativeTimeCompact formats recent times for dense TUI tables, for example
// "2h ago", "3d ago", or "1wk 2d ago".
func RelativeTimeCompact(t time.Time) string {
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
		return compactAgo(int(d/time.Minute), "m")
	case d < 24*time.Hour:
		return compactAgo(int(d/time.Hour), "h")
	case d < 7*24*time.Hour:
		return compactAgo(int(d/(24*time.Hour)), "d")
	case d < 30*24*time.Hour:
		weeks := int(d / (7 * 24 * time.Hour))
		days := int((d % (7 * 24 * time.Hour)) / (24 * time.Hour))
		if days > 0 {
			return strconv.Itoa(weeks) + "wk " + strconv.Itoa(days) + "d ago"
		}
		return compactAgo(weeks, "wk")
	default:
		months := int(d / (30 * 24 * time.Hour))
		return compactAgo(months, "mo")
	}
}

func compactAgo(n int, unit string) string {
	return strconv.Itoa(n) + unit + " ago"
}
