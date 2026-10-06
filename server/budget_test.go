package server

import (
	"math"
	"testing"
	"time"
)

func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLedger_PollsAddDeltasToToday(t *testing.T) {
	l, err := openLedger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	l.record("a", 1.0, now)
	l.record("a", 1.5, now.Add(30*time.Second))
	l.record("b", 2.0, now.Add(30*time.Second))
	near(t, l.today(now), 3.5)
}

func TestLedger_RestartKeepsTotalAndDoesNotRecount(t *testing.T) {
	dir := t.TempDir()
	l, _ := openLedger(dir)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	l.record("a", 1.0, now)
	if err := l.save(now); err != nil {
		t.Fatal(err)
	}

	l, err := openLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	near(t, l.today(now), 1.0)
	l.record("a", 1.25, now.Add(30*time.Second))
	near(t, l.today(now), 1.25)
}

func TestLedger_DeltaSpanningMidnightSplits(t *testing.T) {
	l, _ := openLedger(t.TempDir())
	before := time.Date(2026, 10, 6, 23, 59, 30, 0, time.Local)
	l.record("a", 1.0, before)
	l.record("a", 2.0, before.Add(60*time.Second)) // 30s each side of midnight
	near(t, l.today(before), 1.5)
	near(t, l.today(before.Add(time.Minute)), 0.5)
}
