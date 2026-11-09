package config

import "time"

// WatchdogConfig is the process-storm watchdog's limits. It is separate from
// cpu-watchdog, which only files tickets about gx's own CPU use.
type WatchdogConfig struct {
	// MaxTreeProcesses is the process count in one agent's tree above which
	// the watchdog stops that agent.
	MaxTreeProcesses int `json:"max-tree-processes"`
	// Interval is how often the watchdog counts.
	Interval time.Duration `json:"-"`
	// UlimitHeadroom is how many processes above the user's current count an
	// agent may fork. The cap is fixed when the agent launches.
	UlimitHeadroom int `json:"ulimit-headroom"`
}

// DefaultWatchdogConfig is watchdog's default.
func DefaultWatchdogConfig() WatchdogConfig {
	return WatchdogConfig{MaxTreeProcesses: 200, Interval: time.Second, UlimitHeadroom: 1000}
}
