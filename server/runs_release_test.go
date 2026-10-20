package server

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func waitingRegistry(t *testing.T, waits ...time.Duration) (*runRegistry, time.Time) {
	t.Helper()
	now := time.Now()
	r, _, err := openRuns(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, ago := range waits {
		run := trackedRun{Root: "proj:epic-a"}
		run.Address = fmt.Sprintf("proj:epic-a/%02d", i+1)
		run.WaitingSince = now.Add(-ago)
		r.put(run)
	}
	return r, now
}

// Seam: after the grace period a waiting run stops counting; the 6th keeps its slot.
func TestReleaseWaiting_StopsCountingAfterGraceUpToCap(t *testing.T) {
	r, now := waitingRegistry(t, time.Hour, time.Hour, time.Hour, time.Hour, time.Hour, time.Hour, time.Minute)
	if got := r.releaseWaiting(now, 10*time.Minute, 5); got != 5 {
		t.Fatalf("released = %d, want 5", got)
	}
	if got := r.releaseWaiting(now, 10*time.Minute, 5); got != 0 {
		t.Errorf("second pass released %d, want 0 at the cap", got)
	}
	// 7 runs: 5 released, the 6th overdue keeps its slot, the 7th is still in grace.
	if got, want := r.count(), 2; got != want {
		t.Errorf("count = %d, want %d", got, want)
	}
	if got := r.countRoot("proj:epic-a"); got != 2 {
		t.Errorf("countRoot = %d, want 2", got)
	}
	if got := r.countProject("proj"); got != 2 {
		t.Errorf("countProject = %d, want 2", got)
	}
}

// Seam: an answer re-takes the slot at once; the flag survives a restart.
func TestReleaseWaiting_AnswerRetakesSlotAndFlagSurvivesRestart(t *testing.T) {
	r, now := waitingRegistry(t, time.Hour, time.Hour)
	if r.releaseWaiting(now, 10*time.Minute, 1) != 1 {
		t.Fatal("want one run released")
	}
	reopened, saved, err := openRuns(filepath.Dir(r.path), nil)
	if err != nil {
		t.Fatal(err)
	}
	released := 0
	for _, s := range saved {
		if s.SlotReleased {
			released++
			reopened.put(s)
		}
	}
	if released != 1 || reopened.count() != 0 {
		t.Fatalf("after restart released = %d, count = %d, want 1 and 0", released, reopened.count())
	}
	for _, s := range saved {
		if s.SlotReleased {
			reopened.clearWaiting(s.Address)
		}
	}
	if got := reopened.count(); got != 1 {
		t.Errorf("count after answer = %d, want 1", got)
	}
}
