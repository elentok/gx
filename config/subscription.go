package config

// SubscriptionConfig holds settings for the subscription extra-usage safety
// check (see the subscription package).
type SubscriptionConfig struct {
	// SuppressExtraUsageWarning permanently silences the enabled-state
	// warning once the operator has acknowledged it.
	SuppressExtraUsageWarning bool `json:"suppress-extra-usage-warning"`
}

// DefaultSubscriptionConfig returns the subscription-check defaults.
func DefaultSubscriptionConfig() SubscriptionConfig {
	return SubscriptionConfig{
		SuppressExtraUsageWarning: false,
	}
}
