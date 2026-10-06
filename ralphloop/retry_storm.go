package ralphloop

import (
	"fmt"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
)

// retryStormLaunches reads the cap on every call, so editing the config takes
// effect on the next failure without restarting the loop. A package var so
// tests can pin it.
var retryStormLaunches = func() int {
	cfg, err := config.Load()
	if err != nil || cfg.ExecutionQueue.RetryStormLaunches < 1 {
		return config.DefaultExecutionQueueConfig().RetryStormLaunches
	}
	return cfg.ExecutionQueue.RetryStormLaunches
}

// consecutiveLaunchFailures counts the failed iterations for ticket since its
// last successful launch. An iteration's first launch-failed event (attempt 1)
// stands for the whole iteration, so in-iteration retries don't inflate the
// count; iteration-started is the success that resets it.
func consecutiveLaunchFailures(evs []Event, ticket string) int {
	n := 0
	for i := len(evs) - 1; i >= 0; i-- {
		ev := evs[i]
		if ev.Ticket != ticket {
			continue
		}
		if ev.Type == eventIterationStarted {
			break
		}
		if ev.Type == string(events.LaunchFailed) && ev.Attempt == 1 {
			n++
		}
	}
	return n
}

// retryStormKind upgrades a launch failure's park kind to retry-exhausted once
// the ticket's consecutive failed launches reach the cap.
func retryStormKind(scratchDir, epicName, ticket string, kind events.Kind, reason string) (events.Kind, string) {
	evs, _, err := ReadEvents(scratchDir, epicName)
	if err != nil {
		return kind, reason
	}
	limit := retryStormLaunches()
	if n := consecutiveLaunchFailures(evs, ticket); n >= limit {
		return events.RetryExhausted, fmt.Sprintf("%d consecutive launch failures (limit %d); last: %s", n, limit, reason)
	}
	return kind, reason
}
