package limits

import "time"

// spendWindow is one spend total. Short is set only when the ledger starts
// after the window's start day; the caller then writes "since <First>".
type spendWindow struct {
	Total float64
	Short bool
}

// spendWindows are the spend totals over the budget ledger. First is the first
// day the ledger has, empty when the ledger is empty.
type spendWindows struct {
	First string
	Today spendWindow
	Week  spendWindow // since the most recent Sunday, today when today is Sunday
	Month spendWindow // since the 1st of the month
}

// computeSpendWindows sums the ledger's day keys (DayLayout, in now's
// zone) from each window's start day through today. Keys are fixed-width
// dates, so string order is date order. Window starts use calendar
// arithmetic, never a 24h subtraction, so a daylight-saving week is exact.
func computeSpendWindows(now time.Time, days map[string]float64) spendWindows {
	y, m, d := now.Date()
	loc := now.Location()
	key := func(t time.Time) string { return t.Format(DayLayout) }

	today := key(now)
	weekStart := key(time.Date(y, m, d-int(now.Weekday()), 0, 0, 0, 0, loc))
	monthStart := key(time.Date(y, m, 1, 0, 0, 0, 0, loc))

	var w spendWindows
	var week, month, tod float64
	for k, v := range days {
		if k > today {
			continue
		}
		if w.First == "" || k < w.First {
			w.First = k
		}
		if k >= monthStart {
			month += v
		}
		if k >= weekStart {
			week += v
		}
		if k == today {
			tod += v
		}
	}
	w.Today = spendWindow{Total: tod}
	w.Week = spendWindow{Total: week, Short: isShort(w.First, weekStart)}
	w.Month = spendWindow{Total: month, Short: isShort(w.First, monthStart)}
	return w
}

// isShort reports whether the ledger starts after the window start.
func isShort(first, start string) bool {
	return first != "" && first > start
}
