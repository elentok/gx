package config

const defaultExecutionQueueConcurrency = 2

// ExecutionQueueConfig controls parallel execution from the Tickets and Queue tabs.
type ExecutionQueueConfig struct {
	MaxConcurrentTicketsPerEpic int `json:"max-concurrent-tickets-per-epic"`
	MaxConcurrentEpics          int `json:"max-concurrent-epics"`
}

// DefaultExecutionQueueConfig returns the execution queue defaults.
func DefaultExecutionQueueConfig() ExecutionQueueConfig {
	return ExecutionQueueConfig{
		MaxConcurrentTicketsPerEpic: defaultExecutionQueueConcurrency,
		MaxConcurrentEpics:          defaultExecutionQueueConcurrency,
	}
}

func clampExecutionQueueLimit(value int) int {
	return max(value, 1)
}
