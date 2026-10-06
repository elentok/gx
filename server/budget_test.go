package server

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBudgetStatus_SnapshotAndBudgetRouteAgree(t *testing.T) {
	// A short path: the test name would push the unix socket path past its limit.
	stateDir, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	s, err := New(Config{StateDir: stateDir,TicketStore: t.TempDir(), BudgetSoftLimit: 300, BudgetHardLimit: 350})
	if err != nil {
		t.Fatal(err)
	}
	s.ledger.record("a", 4.25, time.Now())

	var snap Snapshot
	rec := httptest.NewRecorder()
	s.snapshot(rec, httptest.NewRequest("GET", "/v1/snapshot", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	var status BudgetStatus
	rec = httptest.NewRecorder()
	s.budget(rec, httptest.NewRequest("GET", "/v1/budget", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}

	if snap.Budget != status {
		t.Fatalf("snapshot budget %+v != budget route %+v", snap.Budget, status)
	}
	near(t, status.Total, 4.25)
	near(t, status.SoftLimit, 300)
	near(t, status.HardLimit, 350)
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
