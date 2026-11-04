package limits

import "time"

// StatsDayLayout is the day label format of the stats ranges.
const StatsDayLayout = DayLayout

// StatsWindow maps instants onto the range's local days, ending today.
type StatsWindow struct {
	Days  []time.Time // local midnight of each day, oldest first
	index map[string]int
}

// NewStatsWindow is the last n local days, ending today.
func NewStatsWindow(n int, now time.Time) StatsWindow {
	w := StatsWindow{index: map[string]int{}}
	for i := n - 1; i >= 0; i-- {
		day := time.Date(now.Year(), now.Month(), now.Day()-i, 0, 0, 0, 0, time.Local)
		w.index[day.Format(StatsDayLayout)] = len(w.Days)
		w.Days = append(w.Days, day)
	}
	return w
}

// Labels is each day as StatsDayLayout.
func (w StatsWindow) Labels() []string {
	out := make([]string, len(w.Days))
	for i, d := range w.Days {
		out[i] = d.Format(StatsDayLayout)
	}
	return out
}

// Bucket is the day index of t in the server's local time zone, or -1 when t is
// unset or outside the range.
func (w StatsWindow) Bucket(t time.Time) int {
	if t.IsZero() {
		return -1
	}
	i, ok := w.index[t.In(time.Local).Format(StatsDayLayout)]
	if !ok {
		return -1
	}
	return i
}
