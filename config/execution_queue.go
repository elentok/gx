package config

import "time"

const (
	defaultExecutionQueueConcurrency = 2
	defaultMaxAgents                 = 4
	defaultRetryStormLaunches        = 3
	defaultSpinCycles                = 3
	defaultSpinWindow                = 5 * time.Minute
)

// ExecutionQueueConfig controls parallel execution from the Tickets and Queue tabs.
type ExecutionQueueConfig struct {
	MaxConcurrentTicketsPerEpic int `json:"max-concurrent-tickets-per-epic"`
	MaxConcurrentEpics          int `json:"max-concurrent-epics"`
	// MaxAgents caps live agents across every project (the daemon's slot cap).
	MaxAgents int `json:"max-agents"`
	// RetryStormLaunches is how many consecutive failed launches park a ticket
	// as retry-exhausted.
	RetryStormLaunches int `json:"retry-storm-launches"`
	// SpinCycles park/re-claim cycles inside SpinWindow quarantine a ticket.
	// SpinWindow is read from the "spin-window" key as a Go duration string
	// (e.g. "5m"); an unparsable or non-positive value keeps the default.
	SpinCycles int           `json:"spin-cycles"`
	SpinWindow time.Duration `json:"-"`
}

// DefaultExecutionQueueConfig returns the execution queue defaults.
func DefaultExecutionQueueConfig() ExecutionQueueConfig {
	return ExecutionQueueConfig{
		MaxConcurrentTicketsPerEpic: defaultExecutionQueueConcurrency,
		MaxConcurrentEpics:          defaultExecutionQueueConcurrency,
		MaxAgents:                   defaultMaxAgents,
		RetryStormLaunches:          defaultRetryStormLaunches,
		SpinCycles:                 defaultSpinCycles,
		SpinWindow:                  defaultSpinWindow,
	}
}

func clampExecutionQueueLimit(value int) int {
	return max(value, 1)
}
