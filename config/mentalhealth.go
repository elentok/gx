package config

import (
	"fmt"

	"github.com/elentok/gx/config/registry"
)

// MentalHealthConfig is config mental-health: bedtime and work-break rules.
// Every key is global-only and off by default.
type MentalHealthConfig struct {
	Enabled bool                `json:"enabled"`
	Bedtime MentalHealthBedtime `json:"bedtime"`
	Break   MentalHealthBreak   `json:"break"`
}

// MentalHealthBedtime is the nightly window. Start and End are 24-hour HH:MM.
// The window may cross midnight; Start never equals End.
type MentalHealthBedtime struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start"`
	End     string `json:"end"`
	// Message may hold {end}.
	Message string `json:"message"`
}

// MentalHealthBreak is the work-time rule. Empty Message means the built-in
// messages.
type MentalHealthBreak struct {
	Enabled      bool   `json:"enabled"`
	WorkMinutes  int    `json:"work-minutes"`
	BreakMinutes int    `json:"break-minutes"`
	Message      string `json:"message"`
}

// Smallest accepted values, shared with the registry entries' minimums.
const (
	minMentalHealthWorkMinutes  = 3
	minMentalHealthBreakMinutes = 1
)

// DefaultMentalHealthConfig is the default: everything off.
func DefaultMentalHealthConfig() MentalHealthConfig {
	return MentalHealthConfig{
		Bedtime: MentalHealthBedtime{
			Start:   "22:00",
			End:     "07:00",
			Message: "That's enough AI for today. Go to sleep and come back tomorrow at {end}.",
		},
		Break: MentalHealthBreak{WorkMinutes: 55, BreakMinutes: 5},
	}
}

type rawMentalHealth struct {
	Enabled *bool `json:"enabled"`
	Bedtime *struct {
		Enabled *bool   `json:"enabled"`
		Start   *string `json:"start"`
		End     *string `json:"end"`
		Message *string `json:"message"`
	} `json:"bedtime"`
	Break *struct {
		Enabled      *bool   `json:"enabled"`
		WorkMinutes  *int    `json:"work-minutes"`
		BreakMinutes *int    `json:"break-minutes"`
		Message      *string `json:"message"`
	} `json:"break"`
}

// applyMentalHealth overlays the file's keys. Like the other numeric keys, a
// value that fails its check is ignored and the default stays; only a Start
// equal to End is an error, because no default can stand in for it.
func applyMentalHealth(cfg MentalHealthConfig, raw *rawMentalHealth) (MentalHealthConfig, error) {
	if raw.Enabled != nil {
		cfg.Enabled = *raw.Enabled
	}
	if b := raw.Bedtime; b != nil {
		if b.Enabled != nil {
			cfg.Bedtime.Enabled = *b.Enabled
		}
		if b.Start != nil && registry.IsClock(*b.Start) {
			cfg.Bedtime.Start = *b.Start
		}
		if b.End != nil && registry.IsClock(*b.End) {
			cfg.Bedtime.End = *b.End
		}
		if b.Message != nil {
			cfg.Bedtime.Message = *b.Message
		}
	}
	if b := raw.Break; b != nil {
		if b.Enabled != nil {
			cfg.Break.Enabled = *b.Enabled
		}
		if b.WorkMinutes != nil && *b.WorkMinutes >= minMentalHealthWorkMinutes {
			cfg.Break.WorkMinutes = *b.WorkMinutes
		}
		if b.BreakMinutes != nil && *b.BreakMinutes >= minMentalHealthBreakMinutes {
			cfg.Break.BreakMinutes = *b.BreakMinutes
		}
		if b.Message != nil {
			cfg.Break.Message = *b.Message
		}
	}
	if cfg.Bedtime.Start == cfg.Bedtime.End {
		return cfg, fmt.Errorf("mental-health.bedtime.start and mental-health.bedtime.end must differ (both are %s)", cfg.Bedtime.Start)
	}
	return cfg, nil
}
