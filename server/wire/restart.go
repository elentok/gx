package wire

// Restart refusal reasons.
const (
	ReasonRestartChanged    = "changed"
	ReasonRestartInProgress = "in_progress"
	ReasonRestartCheck      = "check_failed"
)

// RestartRequest carries the values the dialog confirmed. Only these decide
// whether the check changed: a new file list or error text does not.
type RestartRequest struct {
	NewVersion      string   `json:"new_version"`
	ConfigOK        bool     `json:"config_ok"`
	FailingProjects []string `json:"failing_projects"`
}

// RestartResult is the restart route's answer. A refusal is a result, not an
// error. Report is the fresh check when the confirmed values went stale.
type RestartResult struct {
	Started    bool                `json:"started"`
	Refused    bool                `json:"refused"`
	Reason     string              `json:"reason,omitempty"`
	Message    string              `json:"message,omitempty"`
	Restarting bool                `json:"restarting,omitempty"`
	Report     *RestartCheckReport `json:"report,omitempty"`
	RestartLog string              `json:"restart_log,omitempty"`
}
