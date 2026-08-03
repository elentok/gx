package tickets

import (
	"errors"
	"testing"
)

func TestTryStartSameEpicTwiceFails(t *testing.T) {
	r := newLoopRegistry(2)

	if _, ok := r.tryStart("epic-a", 0, 5); !ok {
		t.Fatalf("first tryStart for epic-a: want ok")
	}
	if _, ok := r.tryStart("epic-a", 0, 5); ok {
		t.Fatalf("second tryStart for epic-a while running: want !ok")
	}
}

func TestTryStartDifferentEpicsUpToCapSucceed(t *testing.T) {
	r := newLoopRegistry(2)

	if _, ok := r.tryStart("epic-a", 0, 5); !ok {
		t.Fatalf("tryStart epic-a: want ok")
	}
	if _, ok := r.tryStart("epic-b", 0, 3); !ok {
		t.Fatalf("tryStart epic-b while epic-a running and under cap: want ok")
	}
	if !r.isRunning() {
		t.Fatalf("isRunning: want true with two epics running")
	}
}

func TestTryStartBeyondCapFailsUntilSlotFrees(t *testing.T) {
	r := newLoopRegistry(2)

	if _, ok := r.tryStart("epic-a", 0, 5); !ok {
		t.Fatalf("tryStart epic-a: want ok")
	}
	if _, ok := r.tryStart("epic-b", 0, 3); !ok {
		t.Fatalf("tryStart epic-b: want ok")
	}
	if _, ok := r.tryStart("epic-c", 0, 1); ok {
		t.Fatalf("tryStart epic-c beyond cap: want !ok")
	}

	r.finish("epic-a", nil)

	if _, ok := r.tryStart("epic-c", 0, 1); !ok {
		t.Fatalf("tryStart epic-c after a slot freed: want ok")
	}
}

func TestFinishTracksEachEpicsErrorIndependently(t *testing.T) {
	r := newLoopRegistry(2)

	if _, ok := r.tryStart("epic-a", 0, 5); !ok {
		t.Fatalf("tryStart epic-a: want ok")
	}
	if _, ok := r.tryStart("epic-b", 0, 3); !ok {
		t.Fatalf("tryStart epic-b: want ok")
	}

	wantErr := errors.New("epic-b failed")
	r.finish("epic-a", nil)
	r.finish("epic-b", wantErr)

	if err := r.takeLastError("epic-a"); err != nil {
		t.Fatalf("takeLastError(epic-a) = %v, want nil", err)
	}
	if err := r.takeLastError("epic-b"); !errors.Is(err, wantErr) {
		t.Fatalf("takeLastError(epic-b) = %v, want %v", err, wantErr)
	}
	// Taken once already; a second take reports nothing further.
	if err := r.takeLastError("epic-b"); err != nil {
		t.Fatalf("second takeLastError(epic-b) = %v, want nil", err)
	}
}

func TestIsRunningReflectsAnyEpic(t *testing.T) {
	r := newLoopRegistry(2)

	if r.isRunning() {
		t.Fatalf("isRunning: want false with nothing started")
	}

	r.tryStart("epic-a", 0, 5)
	if !r.isRunning() {
		t.Fatalf("isRunning: want true with epic-a running")
	}

	r.tryStart("epic-b", 0, 3)
	r.finish("epic-a", nil)
	if !r.isRunning() {
		t.Fatalf("isRunning: want true with epic-b still running")
	}

	r.finish("epic-b", nil)
	if r.isRunning() {
		t.Fatalf("isRunning: want false once every epic has finished")
	}
}

func TestSnapshotPrefersRequestedEpic(t *testing.T) {
	r := newLoopRegistry(2)
	r.tryStart("epic-a", 1, 5)
	r.tryStart("epic-b", 2, 3)

	running, epicName, _ := r.snapshot("epic-b")
	if !running || epicName != "epic-b" {
		t.Fatalf("snapshot(epic-b) = (%v, %q), want (true, epic-b)", running, epicName)
	}

	r.finish("epic-b", nil)
	running, epicName, _ = r.snapshot("epic-b")
	if !running || epicName != "epic-a" {
		t.Fatalf("snapshot(epic-b) after it finished = (%v, %q), want fallback (true, epic-a)", running, epicName)
	}

	r.finish("epic-a", nil)
	running, _, events := r.snapshot("epic-b")
	if running || events != nil {
		t.Fatalf("snapshot with nothing running: want (false, nil events)")
	}
}
