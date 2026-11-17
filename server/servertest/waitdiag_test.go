package servertest

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fatalRecorder is a testing.TB whose Fatalf records instead of stopping.
type fatalRecorder struct {
	testing.TB
	failures []string
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func TestWaitState_FailurePrintsTheLastStateRead(t *testing.T) {
	t.Parallel()
	rec := &fatalRecorder{TB: t}
	reads := 0
	WaitState(rec, 50*time.Millisecond, "the gate", func() (bool, string) {
		reads++
		return false, fmt.Sprintf("verdict #%d", reads)
	})
	if len(rec.failures) != 1 || !strings.Contains(rec.failures[0], "timed out waiting for the gate") ||
		!strings.Contains(rec.failures[0], fmt.Sprintf("verdict #%d", reads)) {
		t.Fatalf("failures = %q, want the wait name and the last state read (#%d)", rec.failures, reads)
	}
}

func TestWaitState_ReturnsOnceDone(t *testing.T) {
	t.Parallel()
	rec := &fatalRecorder{TB: t}
	WaitState(rec, time.Second, "now", func() (bool, string) { return true, "" })
	if len(rec.failures) != 0 {
		t.Fatalf("failures = %q, want none", rec.failures)
	}
}

func TestWaitForSession_GiveUpPrintsTheEntryAndServerLog(t *testing.T) {
	t.Parallel()
	h := Start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, failure := waitForSession(ctx, h, evs, "no-such-session", nil)
	for _, want := range []string{"no-such-session", "last session entry read:", "server log tail:"} {
		if !strings.Contains(failure, want) {
			t.Errorf("failure = %q, want it to contain %q", failure, want)
		}
	}
}
