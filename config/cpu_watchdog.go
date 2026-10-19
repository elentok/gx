package config

import "time"

// CPUWatchdogConfig is when gx files a ticket to investigate its own CPU use.
// Filing needs evolve maintainer mode (a registered gx project); without it a
// spike is only logged.
type CPUWatchdogConfig struct {
	Enabled bool `json:"enabled"`
	// ThresholdPercent is the CPU use, in percent of one core, that counts as
	// high, for the server and each TUI on its own.
	ThresholdPercent float64 `json:"threshold-percent"`
	// Sustain is how long the use must stay high, and low before a fired
	// spike re-arms.
	Sustain time.Duration `json:"-"`
	// Cooldown is the least time between two spikes: each process captures
	// evidence at most this often, and the server files tickets at most this
	// often.
	Cooldown time.Duration `json:"-"`
}

// DefaultCPUWatchdogConfig is cpu-watchdog's default.
func DefaultCPUWatchdogConfig() CPUWatchdogConfig {
	return CPUWatchdogConfig{Enabled: true, ThresholdPercent: 15, Sustain: time.Minute, Cooldown: 6 * time.Hour}
}
