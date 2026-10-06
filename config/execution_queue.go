package config

const (
	defaultExecutionQueueConcurrency = 2
	defaultRetryStormLaunches        = 3
)

// ExecutionQueueConfig controls parallel execution from the Tickets and Queue tabs.
type ExecutionQueueConfig struct {
	MaxConcurrentTicketsPerEpic int `json:"max-concurrent-tickets-per-epic"`
	MaxConcurrentEpics          int `json:"max-concurrent-epics"`
	// RetryStormLaunches is how many consecutive failed launches park a ticket
	// as retry-exhausted.
	RetryStormLaunches int `json:"retry-storm-launches"`
}

// DefaultExecutionQueueConfig returns the execution queue defaults.
func DefaultExecutionQueueConfig() ExecutionQueueConfig {
	return ExecutionQueueConfig{
		MaxConcurrentTicketsPerEpic: defaultExecutionQueueConcurrency,
		MaxConcurrentEpics:          defaultExecutionQueueConcurrency,
		RetryStormLaunches:          defaultRetryStormLaunches,
	}
}

func clampExecutionQueueLimit(value int) int {
	return max(value, 1)
}
