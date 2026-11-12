package schedules

import (
	"testing"
	"time"
	_ "time/tzdata"
)

var allDays = []time.Weekday{0, 1, 2, 3, 4, 5, 6}

func loadLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func at(loc *time.Location, y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, loc)
}

func TestNext(t *testing.T) {
	utc := time.UTC
	ny := loadLoc(t, "America/New_York")
	berlin := loadLoc(t, "Europe/Berlin")
	// 2026-01-05 is a Monday.
	mon := Schedule{Days: []time.Weekday{time.Monday, time.Wednesday}, Times: []string{"09:00"}}
	multi := Schedule{Days: allDays, Times: []string{"08:00", "12:30", "23:59"}}
	midnight := Schedule{Days: allDays, Times: []string{"00:00"}}

	tests := []struct {
		name  string
		s     Schedule
		after time.Time
		want  time.Time
	}{
		{"weekday filter skips unset days", mon, at(utc, 2026, 1, 5, 9, 30), at(utc, 2026, 1, 7, 9, 0)},
		{"weekday filter wraps the week", mon, at(utc, 2026, 1, 7, 10, 0), at(utc, 2026, 1, 12, 9, 0)},
		{"after exactly on a set time is excluded", mon, at(utc, 2026, 1, 5, 9, 0), at(utc, 2026, 1, 7, 9, 0)},
		{"same day later time", multi, at(utc, 2026, 1, 5, 9, 0), at(utc, 2026, 1, 5, 12, 30)},
		{"after the last time rolls to the next day", multi, at(utc, 2026, 1, 5, 23, 59), at(utc, 2026, 1, 6, 8, 0)},
		{"midnight edge", midnight, at(utc, 2026, 1, 5, 23, 59), at(utc, 2026, 1, 6, 0, 0)},
		{"year end", midnight, at(utc, 2026, 12, 31, 0, 0), at(utc, 2027, 1, 1, 0, 0)},
		{"spring gap runs at 03:00", Schedule{Days: allDays, Times: []string{"02:30"}},
			at(ny, 2026, 3, 8, 0, 0), at(ny, 2026, 3, 8, 3, 0)},
		{"spring gap keeps the nominal weekday", Schedule{Days: []time.Weekday{time.Sunday}, Times: []string{"02:30"}},
			at(ny, 2026, 3, 2, 0, 0), at(ny, 2026, 3, 8, 3, 0)},
		{"spring gap time and a real 03:00 collapse", Schedule{Days: allDays, Times: []string{"02:30", "03:00"}},
			at(ny, 2026, 3, 8, 0, 0), at(ny, 2026, 3, 8, 3, 0)},
		{"after the collapsed time moves to the next day", Schedule{Days: allDays, Times: []string{"02:30", "03:00"}},
			at(ny, 2026, 3, 8, 3, 0), at(ny, 2026, 3, 9, 2, 30)},
		{"fall repeat runs at the first occurrence", Schedule{Days: allDays, Times: []string{"01:30"}},
			at(ny, 2026, 11, 1, 0, 0), at(ny, 2026, 11, 1, 1, 30).UTC().In(ny)},
		{"berlin spring gap", Schedule{Days: allDays, Times: []string{"02:30"}},
			at(berlin, 2026, 3, 29, 0, 0), at(berlin, 2026, 3, 29, 3, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Next(tt.s, tt.after)
			if !got.Equal(tt.want) {
				t.Fatalf("Next = %v, want %v", got, tt.want)
			}
			if back := LatestDue(tt.s, tt.after, got); !back.Equal(got) {
				t.Fatalf("LatestDue(after, Next) = %v, want %v", back, got)
			}
		})
	}
}

func TestNext_FallRepeatIsFirstOccurrence(t *testing.T) {
	ny := loadLoc(t, "America/New_York")
	s := Schedule{Days: allDays, Times: []string{"01:30"}}
	got := Next(s, at(ny, 2026, 11, 1, 0, 0))
	// First occurrence is still daylight time (UTC-4): 05:30 UTC.
	if want := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("Next = %v, want %v", got.UTC(), want)
	}
	// The second 01:30 (06:30 UTC) is not a run: next is the following day.
	next := Next(s, got)
	if want := at(ny, 2026, 11, 2, 1, 30); !next.Equal(want) {
		t.Fatalf("Next after first occurrence = %v, want %v", next, want)
	}
}

func TestLatestDue(t *testing.T) {
	utc := time.UTC
	ny := loadLoc(t, "America/New_York")
	multi := Schedule{Days: allDays, Times: []string{"08:00", "12:30", "23:59"}}
	spring := Schedule{Days: allDays, Times: []string{"02:30"}}
	fall := Schedule{Days: allDays, Times: []string{"01:30"}}

	tests := []struct {
		name       string
		s          Schedule
		after, now time.Time
		want       time.Time // zero = none
	}{
		{"latest of three missed", multi, at(utc, 2026, 1, 5, 7, 0), at(utc, 2026, 1, 6, 9, 0), at(utc, 2026, 1, 6, 8, 0)},
		{"older missed times are ignored", multi, at(utc, 2026, 1, 1, 0, 0), at(utc, 2026, 1, 3, 13, 0), at(utc, 2026, 1, 3, 12, 30)},
		{"now exactly on a set time is included", multi, at(utc, 2026, 1, 5, 7, 0), at(utc, 2026, 1, 5, 8, 0), at(utc, 2026, 1, 5, 8, 0)},
		{"after exactly on a set time is excluded", multi, at(utc, 2026, 1, 5, 8, 0), at(utc, 2026, 1, 5, 12, 29), time.Time{}},
		{"none due", multi, at(utc, 2026, 1, 5, 8, 1), at(utc, 2026, 1, 5, 12, 0), time.Time{}},
		{"weekday filter", Schedule{Days: []time.Weekday{time.Monday}, Times: []string{"09:00"}},
			at(utc, 2026, 1, 5, 9, 0), at(utc, 2026, 1, 11, 23, 0), time.Time{}},
		{"midnight and year end", Schedule{Days: allDays, Times: []string{"00:00"}},
			at(utc, 2026, 12, 30, 12, 0), at(utc, 2027, 1, 1, 0, 0), at(utc, 2027, 1, 1, 0, 0)},
		{"spring gap not yet due before 03:00", spring, at(ny, 2026, 3, 7, 3, 0), at(ny, 2026, 3, 8, 1, 59), time.Time{}},
		{"spring gap due at 03:00", spring, at(ny, 2026, 3, 7, 3, 0), at(ny, 2026, 3, 8, 3, 0), at(ny, 2026, 3, 8, 3, 0)},
		{"spring gap collapse with real 03:00", Schedule{Days: allDays, Times: []string{"02:30", "03:00"}},
			at(ny, 2026, 3, 7, 12, 0), at(ny, 2026, 3, 8, 5, 0), at(ny, 2026, 3, 8, 3, 0)},
		{"spring gap run is not due twice", spring, at(ny, 2026, 3, 8, 3, 0), at(ny, 2026, 3, 8, 12, 0), time.Time{}},
		{"fall repeat runs once", fall, at(ny, 2026, 11, 1, 0, 0), at(ny, 2026, 11, 1, 3, 0),
			time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)},
		{"fall repeat second occurrence is not due", fall, time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC),
			at(ny, 2026, 11, 1, 3, 0), time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LatestDue(tt.s, tt.after, tt.now)
			if !got.Equal(tt.want) {
				t.Fatalf("LatestDue = %v, want %v", got, tt.want)
			}
		})
	}
}
