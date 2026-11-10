package wire

// Status values of a TargetResult.
const (
	TargetDone    = "done"
	TargetRefused = "refused"
	// TargetFits is a dry run's answer for a target that would be done.
	TargetFits = "fits"
)

// TargetResult is what one target of a many-target write came to. A refusal is
// a result, not an error (ADR 0028), and blocks no other target.
type TargetResult struct {
	Target string `json:"target"`
	Status string `json:"status"`
	// Reason is a stable code, set only when refused.
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	// NewAddress is set only after a rename.
	NewAddress string `json:"new_address,omitempty"`
	// Sessions are the live Session ids the write stopped, or on a dry run would stop.
	Sessions []string `json:"sessions,omitempty"`
}

// Refused reports whether the target was refused.
func (r TargetResult) Refused() bool { return r.Status == TargetRefused }

// TargetsResult is the envelope of a many-target write, in argument order.
type TargetsResult struct {
	Results []TargetResult `json:"results"`
}

// RefusedTarget is the result of a target the write refused.
func RefusedTarget(target, reason, message string) TargetResult {
	return TargetResult{Target: target, Status: TargetRefused, Reason: reason, Message: message}
}

// ReasonNeedsYes refuses a target whose write stops live Sessions and came without yes.
const ReasonNeedsYes = "needs-yes"
