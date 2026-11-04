package server

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
)

func withLocalZone(t *testing.T, loc *time.Location) {
	t.Helper()
	old := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = old })
}

func newYork(t *testing.T) *time.Location {
	t.Helper()
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	return ny
}

// ls builds a sample received at the given local time.
func ls(used float64, resets time.Time, y int, m time.Month, d, h, min int) LimitSample {
	return LimitSample{
		UsedPercentage: used,
		ResetsAt:       resets.Unix(),
		ReceivedAt:     time.Date(y, m, d, h, min, 0, 0, time.Local).Unix(),
	}
}

func TestLimitBurnDays(t *testing.T) {
	withLocalZone(t, time.FixedZone("test", 2*3600))
	r1 := time.Date(2026, 10, 12, 0, 0, 0, 0, time.Local)
	r2 := r1.Add(7 * 24 * time.Hour)
	r0 := r1.Add(-7 * 24 * time.Hour)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)

	cases := []struct {
		name    string
		samples []LimitSample
		want    map[string]float64
		resets  []string
	}{
		{
			name: "rises inside one window are credited to the later sample's day",
			samples: []LimitSample{
				ls(10, r1, 2026, 10, 4, 9, 0), ls(15, r1, 2026, 10, 4, 18, 0),
				ls(20, r1, 2026, 10, 5, 8, 0), ls(30, r1, 2026, 10, 5, 20, 0),
			},
			want: map[string]float64{"2026-10-04": 5, "2026-10-05": 15},
		},
		{
			name: "a fall is ignored and does not lower the baseline",
			samples: []LimitSample{
				ls(40, r1, 2026, 10, 5, 8, 0), ls(30, r1, 2026, 10, 5, 9, 0),
				ls(45, r1, 2026, 10, 5, 10, 0),
			},
			want: map[string]float64{"2026-10-05": 5},
		},
		{
			name: "the first sample ever adds nothing",
			samples: []LimitSample{
				ls(60, r1, 2026, 10, 5, 8, 0),
			},
			want: map[string]float64{},
		},
		{
			name: "the first sample after a reset counts in full on its day",
			samples: []LimitSample{
				ls(90, r1, 2026, 10, 5, 8, 0), ls(95, r1, 2026, 10, 5, 22, 0),
				ls(3, r2, 2026, 10, 6, 1, 0), ls(8, r2, 2026, 10, 6, 9, 0),
			},
			want:   map[string]float64{"2026-10-05": 5, "2026-10-06": 8},
			resets: []string{"2026-10-06"},
		},
		{
			name: "a reset time moving backward is dropped",
			samples: []LimitSample{
				ls(10, r1, 2026, 10, 5, 8, 0), ls(50, r0, 2026, 10, 5, 9, 0),
				ls(12, r1, 2026, 10, 5, 10, 0),
			},
			want: map[string]float64{"2026-10-05": 2},
		},
		{
			name: "an ended window keeps its burn and the next window counts in full",
			samples: []LimitSample{
				ls(10, r0, 2026, 10, 4, 8, 0), ls(40, r0, 2026, 10, 4, 20, 0),
				ls(5, r1, 2026, 10, 5, 8, 0),
			},
			want:   map[string]float64{"2026-10-04": 30, "2026-10-05": 5},
			resets: []string{"2026-10-05"},
		},
		{
			name: "a gap credits the later day, never the gap",
			samples: []LimitSample{
				ls(10, r1, 2026, 10, 2, 8, 0), ls(40, r1, 2026, 10, 6, 8, 0),
			},
			want: map[string]float64{"2026-10-06": 30},
		},
		{
			name: "23:59 and 00:01 land on different days",
			samples: []LimitSample{
				ls(10, r1, 2026, 10, 4, 23, 0), ls(20, r1, 2026, 10, 4, 23, 59),
				ls(25, r1, 2026, 10, 5, 0, 1),
			},
			want: map[string]float64{"2026-10-04": 10, "2026-10-05": 5},
		},
		{
			name: "5-hour windows of one day add up and can pass 100",
			samples: []LimitSample{
				ls(0, r1, 2026, 10, 5, 0, 0), ls(80, r1, 2026, 10, 5, 4, 0),
				ls(2, r2, 2026, 10, 5, 6, 0), ls(70, r2, 2026, 10, 5, 10, 0),
				ls(1, r2.Add(time.Hour), 2026, 10, 5, 12, 0), ls(60, r2.Add(time.Hour), 2026, 10, 5, 16, 0),
			},
			want:   map[string]float64{"2026-10-05": 80 + 2 + 68 + 1 + 59},
			resets: []string{"2026-10-05"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			days := limitBurnDays(tc.samples, 7, now)
			resets := map[string]bool{}
			for _, r := range tc.resets {
				resets[r] = true
			}
			for _, d := range days {
				if d.Burn != tc.want[d.Date] {
					t.Errorf("%s burn = %v, want %v", d.Date, d.Burn, tc.want[d.Date])
				}
				if d.ResetDay != resets[d.Date] {
					t.Errorf("%s reset_day = %v, want %v", d.Date, d.ResetDay, resets[d.Date])
				}
			}
		})
	}
}

func TestLimitBurnDaysNoData(t *testing.T) {
	withLocalZone(t, time.FixedZone("test", 2*3600))
	r := time.Date(2026, 10, 12, 0, 0, 0, 0, time.Local)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	samples := []LimitSample{ls(10, r, 2026, 10, 5, 8, 0), ls(20, r, 2026, 10, 5, 9, 0)}

	got := limitBurnDays(samples, 4, now)
	wantNoData := []bool{true, false, false, false} // 04 | 05 06 07
	for i, d := range got {
		if d.NoData != wantNoData[i] {
			t.Errorf("%s no_data = %v, want %v", d.Date, d.NoData, wantNoData[i])
		}
	}
	if got[2].Burn != 0 || got[2].NoData {
		t.Errorf("a quiet day after the first sample is 0, not no data: %+v", got[2])
	}
	for _, d := range limitBurnDays(nil, 3, now) {
		if !d.NoData {
			t.Errorf("no samples: %s should be no data", d.Date)
		}
	}
}

func TestLimitBurnDaysDSTWeek(t *testing.T) {
	ny := newYork(t)
	withLocalZone(t, ny)
	r := time.Date(2026, 11, 10, 0, 0, 0, 0, ny)
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, ny)
	// Fall back is Sunday 2026-11-01: that local day has 25 hours.
	samples := []LimitSample{
		ls(10, r, 2026, 10, 31, 23, 30),
		ls(15, r, 2026, 11, 1, 0, 30),
		ls(20, r, 2026, 11, 1, 23, 30),
		ls(26, r, 2026, 11, 2, 0, 30),
	}
	days := limitBurnDays(samples, 4, now)
	want := map[string]float64{"2026-11-01": 10, "2026-11-02": 6}
	if len(days) != 4 {
		t.Fatalf("got %d days", len(days))
	}
	for _, d := range days {
		if d.Burn != want[d.Date] {
			t.Errorf("%s burn = %v, want %v", d.Date, d.Burn, want[d.Date])
		}
	}
	if days[0].Date != "2026-10-30" || days[3].Date != "2026-11-02" {
		t.Errorf("dates = %v ... %v", days[0].Date, days[3].Date)
	}
}

func TestLimitBurnChip(t *testing.T) {
	withLocalZone(t, time.FixedZone("test", 2*3600))
	r := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)

	t.Run("absent while yesterday is not a full day since the first sample", func(t *testing.T) {
		for _, first := range []LimitSample{
			ls(5, r, 2026, 10, 6, 0, 1), // yesterday started before it
			ls(5, r, 2026, 10, 7, 8, 0), // first sample today
		} {
			if c := limitBurnChip([]LimitSample{first, ls(9, r, 2026, 10, 7, 9, 0)}, now); c != nil {
				t.Errorf("chip = %+v, want nil", c)
			}
		}
		if c := limitBurnChip(nil, now); c != nil {
			t.Errorf("no samples: chip = %+v", c)
		}
	})

	cases := []struct {
		name    string
		samples []LimitSample
		want    wire.LimitBurnChip
	}{
		{
			name:    "present once yesterday starts at the first sample; above yesterday",
			samples: []LimitSample{ls(5, r, 2026, 10, 6, 0, 0), ls(8, r, 2026, 10, 6, 9, 0), ls(20, r, 2026, 10, 7, 9, 0)},
			want:    wire.LimitBurnChip{Today: 12, Yesterday: 3, Direction: wire.LimitChipUp},
		},
		{
			name:    "below yesterday",
			samples: []LimitSample{ls(5, r, 2026, 10, 5, 8, 0), ls(30, r, 2026, 10, 6, 9, 0), ls(33, r, 2026, 10, 7, 9, 0)},
			want:    wire.LimitBurnChip{Today: 3, Yesterday: 25, Direction: wire.LimitChipDown},
		},
		{
			name:    "equal after rounding",
			samples: []LimitSample{ls(5, r, 2026, 10, 5, 8, 0), ls(9.6, r, 2026, 10, 6, 9, 0), ls(14.2, r, 2026, 10, 7, 9, 0)},
			want:    wire.LimitBurnChip{Today: 5, Yesterday: 5, Direction: wire.LimitChipEqual},
		},
		{
			name:    "fractional burn reaches the chip as whole points",
			samples: []LimitSample{ls(5, r, 2026, 10, 5, 8, 0), ls(9.6, r, 2026, 10, 6, 9, 0), ls(17, r, 2026, 10, 7, 9, 0)},
			want:    wire.LimitBurnChip{Today: 7, Yesterday: 5, Direction: wire.LimitChipUp},
		},
		{
			name:    "a quiet yesterday counts 0",
			samples: []LimitSample{ls(5, r, 2026, 10, 5, 8, 0), ls(9, r, 2026, 10, 7, 9, 0)},
			want:    wire.LimitBurnChip{Today: 4, Yesterday: 0, Direction: wire.LimitChipUp},
		},
		{
			name:    "no burn today yet is 0, still shown",
			samples: []LimitSample{ls(5, r, 2026, 10, 5, 8, 0), ls(9, r, 2026, 10, 6, 9, 0)},
			want:    wire.LimitBurnChip{Today: 0, Yesterday: 4, Direction: wire.LimitChipDown},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := limitBurnChip(tc.samples, now)
			if c == nil || *c != tc.want {
				t.Errorf("chip = %+v, want %+v", c, tc.want)
			}
		})
	}
}

func TestLimitPace(t *testing.T) {
	withLocalZone(t, time.FixedZone("test", 2*3600))
	r := time.Date(2026, 10, 30, 0, 0, 0, 0, time.Local)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local)

	t.Run("divides by the days available and counts quiet days as 0", func(t *testing.T) {
		// First sample at the start of 10-16: full days 16, 17, 18, 19.
		samples := []LimitSample{
			ls(0, r, 2026, 10, 16, 0, 0), ls(8, r, 2026, 10, 16, 9, 0),
			ls(20, r, 2026, 10, 18, 9, 0),
			ls(24, r, 2026, 10, 20, 9, 0), // today: excluded
		}
		pace, days := limitPace(samples, now)
		if days != 4 || pace != 5 { // (8 + 0 + 12 + 0) / 4
			t.Errorf("pace, days = %v, %v; want 5, 4", pace, days)
		}
	})
	t.Run("capped at 7 days", func(t *testing.T) {
		samples := []LimitSample{
			ls(0, r, 2026, 10, 1, 0, 0), ls(100, r, 2026, 10, 2, 9, 0), // older than 7 days: ignored
			ls(107, r, 2026, 10, 15, 9, 0), ls(114, r, 2026, 10, 19, 9, 0),
		}
		pace, days := limitPace(samples, now)
		if days != 7 || pace != 2 { // 14 over days 13..19
			t.Errorf("pace, days = %v, %v; want 2, 7", pace, days)
		}
	})
	t.Run("first sample mid-day: that day is not full", func(t *testing.T) {
		samples := []LimitSample{ls(0, r, 2026, 10, 18, 9, 0), ls(6, r, 2026, 10, 19, 9, 0)}
		pace, days := limitPace(samples, now)
		if days != 1 || pace != 6 {
			t.Errorf("pace, days = %v, %v; want 6, 1", pace, days)
		}
	})
}

func TestLimitForecast(t *testing.T) {
	withLocalZone(t, time.FixedZone("test", 2*3600))
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local)
	reset := now.Add(2 * 24 * time.Hour) // two days left
	past := now.Add(-time.Hour)

	// 4 full days of history at 10 points a day.
	history := func(r time.Time) []LimitSample {
		return []LimitSample{
			ls(0, r, 2026, 10, 16, 0, 0), ls(10, r, 2026, 10, 16, 9, 0), ls(20, r, 2026, 10, 17, 9, 0),
			ls(30, r, 2026, 10, 18, 9, 0), ls(40, r, 2026, 10, 19, 9, 0),
		}
	}
	latest := func(used float64, r time.Time) LimitSample {
		return LimitSample{UsedPercentage: used, ResetsAt: r.Unix(), ReceivedAt: now.Unix()}
	}
	short := []LimitSample{ls(0, reset, 2026, 10, 19, 0, 0), ls(10, reset, 2026, 10, 19, 9, 0)}

	cases := []struct {
		name    string
		samples []LimitSample
		latest  LimitSample
		want    wire.LimitForecast
	}{
		{
			name:    "projects used plus pace times time left",
			samples: history(reset), latest: latest(50, reset),
			want: wire.LimitForecast{Status: wire.LimitForecastProjected, Pace: 10, AtReset: 70},
		},
		{
			name:    "over 100 gives the time the limit runs out",
			samples: history(reset), latest: latest(90, reset),
			want: wire.LimitForecast{
				Status: wire.LimitForecastProjected, Pace: 10, AtReset: 110,
				RunsOutAt: now.Add(24 * time.Hour).Unix(), // (100-90)/10 days
			},
		},
		{
			name:    "exactly 100 at reset does not run out",
			samples: history(reset), latest: latest(80, reset),
			want: wire.LimitForecast{Status: wire.LimitForecastProjected, Pace: 10, AtReset: 100},
		},
		{
			name:    "at or above 100 gets no forecast",
			samples: history(reset), latest: latest(100, reset),
			want: wire.LimitForecast{Status: wire.LimitForecastNone, Pace: 10},
		},
		{
			name:    "a past-reset row gets none, ahead of needs-data",
			samples: short, latest: latest(10, past),
			want: wire.LimitForecast{Status: wire.LimitForecastNone, Pace: 10},
		},
		{
			name:    "a past-reset row with long history gets none",
			samples: history(past), latest: latest(50, past),
			want: wire.LimitForecast{Status: wire.LimitForecastNone, Pace: 10},
		},
		{
			name:    "under 3 days needs data",
			samples: short, latest: latest(10, reset),
			want: wire.LimitForecast{Status: wire.LimitForecastNeedsData, Pace: 10},
		},
		{
			name:    "a stale row with under 3 days needs data",
			samples: short, latest: LimitSample{UsedPercentage: 10, ResetsAt: reset.Unix(), ReceivedAt: now.Add(-48 * time.Hour).Unix()},
			want: wire.LimitForecast{Status: wire.LimitForecastNeedsData, Pace: 10},
		},
		{
			name:    "a stale row with history still projects from the last known value",
			samples: history(reset), latest: LimitSample{UsedPercentage: 50, ResetsAt: reset.Unix(), ReceivedAt: now.Add(-48 * time.Hour).Unix()},
			want: wire.LimitForecast{Status: wire.LimitForecastProjected, Pace: 10, AtReset: 70},
		},
		{
			name: "pace 0 shows the current percentage and no run-out",
			samples: []LimitSample{
				ls(40, reset, 2026, 10, 16, 0, 0), ls(40, reset, 2026, 10, 19, 9, 0),
			},
			latest: latest(95.4, reset),
			want:   wire.LimitForecast{Status: wire.LimitForecastProjected, Pace: 0, AtReset: 95},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := limitForecast(tc.samples, tc.latest, now); got != tc.want {
				t.Errorf("forecast = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestLimitBurnCodexSamples(t *testing.T) {
	withLocalZone(t, time.FixedZone("test", 2*3600))
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	r := time.Date(2026, 10, 9, 0, 0, 0, 0, time.Local)
	samples := []LimitSample{
		{LimitID: LimitIDCodexWeekly, UsedPercentage: 10, ResetsAt: r.Unix(), ReceivedAt: time.Date(2026, 10, 6, 0, 0, 0, 0, time.Local).Unix()},
		{LimitID: LimitIDCodexWeekly, UsedPercentage: 14, ResetsAt: r.Unix(), ReceivedAt: time.Date(2026, 10, 6, 9, 0, 0, 0, time.Local).Unix()},
	}
	if c := limitBurnChip(samples, now); c == nil || c.Yesterday != 4 || c.Today != 0 {
		t.Errorf("codex chip = %+v", c)
	}
}
