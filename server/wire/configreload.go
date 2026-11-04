package wire

// ReloadResult is the outcome of one config reload. A refusal is a result
// (Refused set), not an error (ADR 0028).
type ReloadResult struct {
	// Changed lists the config keys whose value changed, in config-key spelling.
	Changed  []string `json:"changed,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Refused  bool     `json:"refused,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Message  string   `json:"message,omitempty"`
}

const (
	ReasonInvalidConfig     = "invalid-config"
	ReasonConfigMissing     = "config-missing"
	ReasonReloadUnavailable = "reload-unavailable"
)
