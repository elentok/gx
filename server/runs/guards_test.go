package runs

import (
	"slices"
	"testing"
	"time"
)

func TestMergeGuard_CloseAndWaitWaitsForRunningMergesAndRefusesNewOnes(t *testing.T) {
	t.Parallel()
	var g MergeGuard
	if !g.Begin() {
		t.Fatal("Begin refused before close")
	}
	if g.CloseAndWait(10 * time.Millisecond) {
		t.Error("CloseAndWait reported done with a merge in flight")
	}
	if g.Begin() {
		t.Error("Begin allowed a merge after close")
	}
	g.End()
	if !g.CloseAndWait(time.Second) {
		t.Error("CloseAndWait timed out after the merge ended")
	}
}

func TestTrunkLocks_TryLockFailsWhileHeldAndLockedListsTrunks(t *testing.T) {
	t.Parallel()
	var l TrunkLocks
	unlock := l.Lock("repo", "main")
	if _, ok := l.TryLock("repo", "main"); ok {
		t.Error("TryLock took a held trunk")
	}
	other, ok := l.TryLock("repo", "dev")
	if !ok {
		t.Fatal("TryLock refused a free trunk")
	}
	other()
	unlock()
	again, ok := l.TryLock("repo", "main")
	if !ok {
		t.Fatal("TryLock refused a released trunk")
	}
	again()
	got := l.Locked()
	slices.Sort(got)
	if want := []string{"dev", "main"}; !slices.Equal(got, want) {
		t.Errorf("Locked = %v, want %v", got, want)
	}
}
