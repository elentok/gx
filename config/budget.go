package config

import "sort"

const (
	defaultBudgetSoftLimit = 300.0
	defaultBudgetHardLimit = 350.0
)

// BudgetConfig controls estimated API-equivalent-cost soft/hard limits and
// notification thresholds, all in dollars.
type BudgetConfig struct {
	SoftLimit              float64   `json:"soft-limit"`
	HardLimit              float64   `json:"hard-limit"`
	NotificationThresholds []float64 `json:"notification-thresholds"`
}

// DefaultBudgetConfig returns the budget defaults.
func DefaultBudgetConfig() BudgetConfig {
	return BudgetConfig{
		SoftLimit:              defaultBudgetSoftLimit,
		HardLimit:              defaultBudgetHardLimit,
		NotificationThresholds: []float64{50, 100, 150, 200, 250},
	}
}

// clampBudget sorts/dedupes thresholds and, when both limits are nonzero,
// bumps the hard limit up to the soft limit if it's at or below it.
// Thresholds have no enforced ordering relationship with either limit.
func clampBudget(cfg BudgetConfig) BudgetConfig {
	cfg.NotificationThresholds = sortDedupeFloats(cfg.NotificationThresholds)
	if cfg.SoftLimit != 0 && cfg.HardLimit != 0 && cfg.HardLimit <= cfg.SoftLimit {
		cfg.HardLimit = cfg.SoftLimit
	}
	return cfg
}

func sortDedupeFloats(values []float64) []float64 {
	if len(values) == 0 {
		return values
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	deduped := sorted[:1]
	for _, v := range sorted[1:] {
		if v != deduped[len(deduped)-1] {
			deduped = append(deduped, v)
		}
	}
	return deduped
}
