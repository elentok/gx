package ralphloop

import (
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
)

func failedLaunch(ticket string, attempt int) Event {
	return Event{Type: string(events.LaunchFailed), Ticket: ticket, Kind: string(events.IterationError), Attempt: attempt}
}

func TestConsecutiveLaunchFailures(t *testing.T) {
	started := Event{Type: string(events.IterationStarted), Ticket: "11"}
	evs := []Event{
		failedLaunch("11", 1), failedLaunch("11", 1),
		started,
		failedLaunch("11", 1), failedLaunch("11", 2), // one iteration, two attempts
		failedLaunch("12", 1), // another ticket never counts
		failedLaunch("11", 1),
	}
	if got := consecutiveLaunchFailures(evs, "11"); got != 2 {
		t.Fatalf("consecutive failures = %d, want 2 (reset by iteration-started, attempts not double counted)", got)
	}
}

func TestRetryStormKind_PausesAtLimit(t *testing.T) {
	scratch := epicScratchDir(t, "epic")
	opts := RunOptions{RetryStormLaunches: 3}

	for i := 1; i <= 3; i++ {
		_ = logEvent(scratch, "epic", failedLaunch("11", 1))
		kind, _ := retryStormKind(scratch, "epic", "11", opts.RetryStormLaunches, events.IterationError, "boom")
		want := events.IterationError
		if i == 3 {
			want = events.RetryExhausted
		}
		if kind != want {
			t.Fatalf("after %d failures kind = %q, want %q", i, kind, want)
		}
	}

	_ = logEvent(scratch, "epic", Event{Type: string(events.IterationStarted), Ticket: "11"})
	if kind, _ := retryStormKind(scratch, "epic", "11", opts.RetryStormLaunches, events.IterationError, "boom"); kind != events.IterationError {
		t.Fatalf("after a successful launch kind = %q, want the original kind", kind)
	}
}

func TestRetryStormKind_ZeroLimitUsesDefaultCap(t *testing.T) {
	scratch := epicScratchDir(t, "epic")
	limit := config.DefaultExecutionQueueConfig().RetryStormLaunches

	for i := 1; i <= limit; i++ {
		_ = logEvent(scratch, "epic", failedLaunch("11", 1))
		kind, _ := retryStormKind(scratch, "epic", "11", RunOptions{}.RetryStormLaunches, events.IterationError, "boom")
		if want := i == limit; (kind == events.RetryExhausted) != want {
			t.Fatalf("after %d failures kind = %q, retry-exhausted want %v", i, kind, want)
		}
	}
}
