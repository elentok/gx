package server

import (
	"os"
	"path/filepath"
	"testing"
)

func lw(pct float64, reset, received int64) *LimitWindow {
	return &LimitWindow{UsedPercentage: pct, ResetsAt: reset, ReceivedAt: received}
}

func TestLimitsStoreMergeBranches(t *testing.T) {
	const now = 1000
	tests := []struct {
		name     string
		stored   *LimitWindow
		in       *LimitWindow
		outcome  LimitOutcome
		want     LimitWindow
		prev     int64
		wantFile bool
	}{
		{"no stored", nil, lw(10, 2000, 0), LimitNew, LimitWindow{10, 2000, now}, 0, true},
		{"same reset higher", lw(10, 2000, 500), lw(30, 2000, 0), LimitChanged, LimitWindow{30, 2000, now}, 500, true},
		{"same reset higher same rounded", lw(10.2, 2000, 500), lw(10.4, 2000, 0), LimitConfirmed, LimitWindow{10.4, 2000, now}, 500, true},
		{"same reset equal", lw(10, 2000, 500), lw(10, 2000, 0), LimitConfirmed, LimitWindow{10, 2000, now}, 500, true},
		{"same reset lower", lw(30, 2000, 500), lw(10, 2000, 0), LimitDropped, LimitWindow{30, 2000, 500}, 500, false},
		{"earlier reset stored future", lw(30, 2000, 500), lw(90, 1500, 0), LimitDropped, LimitWindow{30, 2000, 500}, 500, false},
		{"earlier reset stored past", lw(30, 900, 500), lw(5, 800, 0), LimitChanged, LimitWindow{5, 800, now}, 500, true},
		{"later reset lower pct", lw(30, 2000, 500), lw(5, 3000, 0), LimitChanged, LimitWindow{5, 3000, now}, 500, true},
		{"stored at reset", lw(30, 1000, 500), lw(5, 1500, 0), LimitChanged, LimitWindow{5, 1500, now}, 500, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			s := openLimitsStore(dir)
			s.state.FiveHour = tc.stored
			res, err := s.apply(LimitsReport{FiveHour: tc.in}, now)
			if err != nil {
				t.Fatal(err)
			}
			if res.FiveHour == nil || res.FiveHour.Outcome != tc.outcome || res.FiveHour.PrevReceivedAt != tc.prev {
				t.Fatalf("result = %+v, want %s prev %d", res.FiveHour, tc.outcome, tc.prev)
			}
			if got := s.read().FiveHour; *got != tc.want {
				t.Fatalf("window = %+v, want %+v", *got, tc.want)
			}
			if res.SevenDay != nil || s.read().SevenDay != nil {
				t.Fatal("seven_day touched by a report without it")
			}
			_, err = os.Stat(filepath.Join(dir, limitsFile))
			if (err == nil) != tc.wantFile {
				t.Fatalf("file present = %v, want %v", err == nil, tc.wantFile)
			}
		})
	}
}

func TestLimitsStoreWindowsMergeIndependently(t *testing.T) {
	s := openLimitsStore(t.TempDir())
	s.state = LimitsReport{FiveHour: lw(50, 2000, 500), SevenDay: lw(10, 9000, 500)}
	res, err := s.apply(LimitsReport{FiveHour: lw(20, 2000, 0), SevenDay: lw(20, 9000, 0)}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if res.FiveHour.Outcome != LimitDropped || res.SevenDay.Outcome != LimitChanged {
		t.Fatalf("result = %+v %+v", res.FiveHour, res.SevenDay)
	}
	got := s.read()
	if *got.FiveHour != *lw(50, 2000, 500) || *got.SevenDay != *lw(20, 9000, 1000) {
		t.Fatalf("state = %+v %+v", got.FiveHour, got.SevenDay)
	}
}

func TestLimitsStoreEmptyReportWritesNothing(t *testing.T) {
	dir := t.TempDir()
	s := openLimitsStore(dir)
	res, err := s.apply(LimitsReport{}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if res.FiveHour != nil || res.SevenDay != nil {
		t.Fatalf("result = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, limitsFile)); !os.IsNotExist(err) {
		t.Fatalf("file written: %v", err)
	}
}

func TestLimitsStorePersistsAcrossOpen(t *testing.T) {
	dir := t.TempDir()
	s := openLimitsStore(dir)
	if _, err := s.apply(LimitsReport{FiveHour: lw(34, 2000, 0), SevenDay: lw(60, 9000, 0)}, 1000); err != nil {
		t.Fatal(err)
	}
	got := openLimitsStore(dir).read()
	if *got.FiveHour != *lw(34, 2000, 1000) || *got.SevenDay != *lw(60, 9000, 1000) {
		t.Fatalf("reloaded = %+v %+v", got.FiveHour, got.SevenDay)
	}
}

func TestLimitsStoreCorruptFileLoadsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, limitsFile), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := openLimitsStore(dir).read(); got.FiveHour != nil || got.SevenDay != nil {
		t.Fatalf("state = %+v", got)
	}
}
