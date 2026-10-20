package config

import "time"

const (
	defaultExecutionQueueConcurrency = 2
	defaultMaxAgents                 = 4
	defaultMaxOneOffAgents           = 3
	defaultRetryStormLaunches        = 3
	defaultSpinCycles                = 3
	defaultSpinWindow                = 5 * time.Minute
	defaultWaitingSlotGrace          = 10 * time.Minute
	defaultMaxReleasedAgents         = 5
	defaultStalledNudgeDelay         = 2 * time.Minute
)

// ExecutionQueueConfig controls parallel execution from the Tickets and Queue tabs.
type ExecutionQueueConfig struct {
	MaxConcurrentTicketsPerEpic int `json:"max-agents-per-epic"`
	// MaxConcurrentEpics is only used by the legacy TUI; it has no config key.
	MaxConcurrentEpics int `json:"-"`
	// MaxAgents caps live agents across every project (the daemon's slot cap).
	MaxAgents int `json:"max-agents"`
	// MaxOneOffAgents caps live agents across one project's one-off tickets.
	MaxOneOffAgents int `json:"max-one-off-agents"`
	// RetryStormLaunches is how many consecutive failed launches park a ticket
	// as retry-exhausted.
	RetryStormLaunches int `json:"retry-storm-launches"`
	// SpinCycles park/re-claim cycles inside SpinWindow quarantine a ticket.
	// SpinWindow is read from the "spin-window" key as a Go duration string
	// (e.g. "5m"); an unparsable or non-positive value keeps the default.
	SpinCycles int           `json:"spin-cycles"`
	SpinWindow time.Duration `json:"-"`
	// WaitingSlotGrace is how long a waiting agent counts toward the slot caps
	// before it releases its slot. Read from the "waiting-slot-grace" key as a
	// Go duration string; an unparsable or non-positive value keeps the default.
	WaitingSlotGrace time.Duration `json:"-"`
	// MaxReleasedAgents caps how many waiting agents may hold a released slot at
	// once, across every project.
	MaxReleasedAgents int `json:"max-released-agents"`
	// StalledNudgeDelay is how long an idle agent with no question and no
	// finished report waits before the server types its one stall nudge. Read
	// from the "stalled-nudge-delay" key as a Go duration string; an unparsable
	// or non-positive value keeps the default.
	StalledNudgeDelay time.Duration `json:"-"`
}

// DefaultExecutionQueueConfig returns the execution queue defaults.
func DefaultExecutionQueueConfig() ExecutionQueueConfig {
	return ExecutionQueueConfig{
		MaxConcurrentTicketsPerEpic: defaultExecutionQueueConcurrency,
		MaxConcurrentEpics:          defaultExecutionQueueConcurrency,
		MaxAgents:                   defaultMaxAgents,
		MaxOneOffAgents:             defaultMaxOneOffAgents,
		RetryStormLaunches:          defaultRetryStormLaunches,
		SpinCycles:                  defaultSpinCycles,
		SpinWindow:                  defaultSpinWindow,
		WaitingSlotGrace:            defaultWaitingSlotGrace,
		MaxReleasedAgents:           defaultMaxReleasedAgents,
		StalledNudgeDelay:           defaultStalledNudgeDelay,
	}
}

func clampExecutionQueueLimit(value int) int {
	return max(value, 1)
}
