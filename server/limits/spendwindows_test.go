package limits

import (
	"testing"
	"time"
)

func TestSpendWindows(t *testing.T) {
	utc := time.UTC
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	at := func(loc *time.Location, y int, m time.Month, d, h, min int) time.Time {
		return time.Date(y, m, d, h, min, 0, 0, loc)
	}

	cases := []struct {
		name string
		now  time.Time
		days map[string]float64
		want spendWindows
	}{
		{
			name: "mid-week, ledger covers everything",
			now:  at(utc, 2026, 10, 14, 12, 0), // Wednesday
			days: map[string]float64{
				"2026-10-01": 1, "2026-10-10": 2, "2026-10-11": 4, "2026-10-13": 8, "2026-10-14": 16,
			},
			want: spendWindows{
				First: "2026-10-01",
				Today: spendWindow{Total: 16},
				Week:  spendWindow{Total: 4 + 8 + 16},
				Month: spendWindow{Total: 31},
			},
		},
		{
			name: "today is Sunday: week starts today",
			now:  at(utc, 2026, 10, 11, 9, 0),
			days: map[string]float64{"2026-10-10": 2, "2026-10-11": 4, "2026-10-01": 1},
			want: spendWindows{
				First: "2026-10-01",
				Today: spendWindow{Total: 4},
				Week:  spendWindow{Total: 4},
				Month: spendWindow{Total: 7},
			},
		},
		{
			name: "today is the 1st: month starts today",
			now:  at(utc, 2026, 10, 1, 9, 0), // Thursday
			days: map[string]float64{"2026-09-30": 2, "2026-10-01": 4, "2026-09-27": 1},
			want: spendWindows{
				First: "2026-09-27",
				Today: spendWindow{Total: 4},
				Week:  spendWindow{Total: 1 + 2 + 4}, // Sunday 09-27 through 10-01
				Month: spendWindow{Total: 4},
			},
		},
		{
			name: "week crosses a month boundary",
			now:  at(utc, 2026, 10, 2, 9, 0), // Friday, Sunday was 09-27
			days: map[string]float64{"2026-09-26": 100, "2026-09-27": 1, "2026-09-30": 2, "2026-10-01": 4, "2026-10-02": 8},
			want: spendWindows{
				First: "2026-09-26",
				Today: spendWindow{Total: 8},
				Week:  spendWindow{Total: 1 + 2 + 4 + 8},
				Month: spendWindow{Total: 4 + 8},
			},
		},
		{
			name: "week crosses a year boundary",
			now:  at(utc, 2027, 1, 2, 9, 0), // Saturday, Sunday was 2026-12-27
			days: map[string]float64{"2026-12-26": 100, "2026-12-27": 1, "2026-12-31": 2, "2027-01-01": 4, "2027-01-02": 8},
			want: spendWindows{
				First: "2026-12-26",
				Today: spendWindow{Total: 8},
				Week:  spendWindow{Total: 1 + 2 + 4 + 8},
				Month: spendWindow{Total: 4 + 8},
			},
		},
		{
			name: "ledger starts mid-window",
			now:  at(utc, 2026, 10, 14, 12, 0),
			days: map[string]float64{"2026-10-13": 8, "2026-10-14": 16},
			want: spendWindows{
				First: "2026-10-13",
				Today: spendWindow{Total: 16},
				Week:  spendWindow{Total: 24, Short: true},
				Month: spendWindow{Total: 24, Short: true},
			},
		},
		{
			name: "first day equal to window start is not short",
			now:  at(utc, 2026, 10, 14, 12, 0),
			days: map[string]float64{"2026-10-11": 4, "2026-10-14": 16},
			want: spendWindows{
				First: "2026-10-11",
				Today: spendWindow{Total: 16},
				Week:  spendWindow{Total: 20},
				Month: spendWindow{Total: 20, Short: true},
			},
		},
		{
			name: "ledger with gaps",
			now:  at(utc, 2026, 10, 14, 12, 0),
			days: map[string]float64{"2026-10-01": 1, "2026-10-12": 2},
			want: spendWindows{
				First: "2026-10-01",
				Today: spendWindow{},
				Week:  spendWindow{Total: 2},
				Month: spendWindow{Total: 3},
			},
		},
		{
			name: "empty ledger",
			now:  at(utc, 2026, 10, 14, 12, 0),
			days: map[string]float64{},
			want: spendWindows{},
		},
		{
			name: "daylight-saving week",
			now:  at(ny, 2026, 3, 11, 12, 0), // Wednesday; clocks sprang forward Sun 03-08
			days: map[string]float64{"2026-03-07": 100, "2026-03-08": 1, "2026-03-09": 2, "2026-03-10": 4, "2026-03-11": 8},
			want: spendWindows{
				First: "2026-03-07",
				Today: spendWindow{Total: 8},
				Week:  spendWindow{Total: 1 + 2 + 4 + 8},
				Month: spendWindow{Total: 115, Short: true},
			},
		},
		{
			name: "daylight-saving fall back, Sunday of the change",
			now:  at(ny, 2026, 11, 1, 23, 30), // Sunday, clocks fell back that day
			days: map[string]float64{"2026-10-31": 100, "2026-11-01": 3},
			want: spendWindows{
				First: "2026-10-31",
				Today: spendWindow{Total: 3},
				Week:  spendWindow{Total: 3},
				Month: spendWindow{Total: 3},
			},
		},
		{
			name: "future keys are excluded",
			now:  at(utc, 2026, 10, 14, 12, 0),
			days: map[string]float64{"2026-10-14": 1, "2026-10-15": 50},
			want: spendWindows{
				First: "2026-10-14",
				Today: spendWindow{Total: 1},
				Week:  spendWindow{Total: 1, Short: true},
				Month: spendWindow{Total: 1, Short: true},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeSpendWindows(tc.now, tc.days)
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
