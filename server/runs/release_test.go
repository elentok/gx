package runs

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func waitingRegistry(t *testing.T, waits ...time.Duration) (*Registry, time.Time) {
	t.Helper()
	now := time.Now()
	r, _, err := Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, ago := range waits {
		run := TrackedRun{Root: "proj:epic-a"}
		run.Address = fmt.Sprintf("proj:epic-a/%02d", i+1)
		run.WaitingSince = now.Add(-ago)
		r.Put(run)
	}
	return r, now
}

// Seam: after the grace period a waiting run stops counting; the 6th keeps its slot.
func TestReleaseWaiting_StopsCountingAfterGraceUpToCap(t *testing.T) {
	t.Parallel()
	r, now := waitingRegistry(t, time.Hour, time.Hour, time.Hour, time.Hour, time.Hour, time.Hour, time.Minute)
	if got := r.ReleaseWaiting(now, 10*time.Minute, 5); got != 5 {
		t.Fatalf("released = %d, want 5", got)
	}
	if got := r.ReleaseWaiting(now, 10*time.Minute, 5); got != 0 {
		t.Errorf("second pass released %d, want 0 at the cap", got)
	}
	// 7 runs: 5 released, the 6th overdue keeps its slot, the 7th is still in grace.
	if got, want := r.Count(), 2; got != want {
		t.Errorf("count = %d, want %d", got, want)
	}
	if got := r.CountRoot("proj:epic-a"); got != 2 {
		t.Errorf("countRoot = %d, want 2", got)
	}
	if got := r.CountProject("proj"); got != 2 {
		t.Errorf("countProject = %d, want 2", got)
	}
}

// Seam: an answer re-takes the slot at once; the flag survives a restart.
func TestReleaseWaiting_AnswerRetakesSlotAndFlagSurvivesRestart(t *testing.T) {
	t.Parallel()
	r, now := waitingRegistry(t, time.Hour, time.Hour)
	if r.ReleaseWaiting(now, 10*time.Minute, 1) != 1 {
		t.Fatal("want one run released")
	}
	reopened, saved, err := Open(filepath.Dir(r.path), nil)
	if err != nil {
		t.Fatal(err)
	}
	released := 0
	for _, s := range saved {
		if s.SlotReleased {
			released++
			reopened.Put(s)
		}
	}
	if released != 1 || reopened.Count() != 0 {
		t.Fatalf("after restart released = %d, count = %d, want 1 and 0", released, reopened.Count())
	}
	for _, s := range saved {
		if s.SlotReleased {
			reopened.ClearWaiting(s.Address)
		}
	}
	if got := reopened.Count(); got != 1 {
		t.Errorf("count after answer = %d, want 1", got)
	}
}
