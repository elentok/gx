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

// RestartCheckOutput is what `gx restart-check` prints. A binary without the
// command exits non-zero with an unknown-command error; one that ran exits zero
// even when the config or a dry run failed (the failure is in the JSON).
type RestartCheckOutput struct {
	Version  string                `json:"version"`
	Config   RestartCheckConfig    `json:"config"`
	Projects []RestartCheckProject `json:"projects"`
}

type RestartCheckConfig struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type RestartCheckProject struct {
	Name  string   `json:"name"`
	OK    bool     `json:"ok"`
	Error string   `json:"error,omitempty"`
	Files []string `json:"files"`
	// Pending is how many migration steps the new binary would still run for
	// the project (its registry minus the project's applied list).
	Pending int `json:"pending"`
}

// Restart check statuses.
const (
	RestartCheckOK         = "ok"
	RestartCheckUnreadable = "unreadable"
	RestartCheckTooOld     = "too_old"
)

// RestartCheckReport is the restart check route's answer. Config and Projects
// are set only when Status is "ok".
type RestartCheckReport struct {
	Status         string                `json:"status"`
	Error          string                `json:"error,omitempty"`
	Hint           string                `json:"hint,omitempty"`
	Binary         string                `json:"binary"`
	RunningVersion string                `json:"running_version"`
	NewVersion     string                `json:"new_version,omitempty"`
	Config         RestartCheckConfig    `json:"config"`
	Projects       []RestartCheckProject `json:"projects"`
	Agents         int                   `json:"agents"`
	Instance       string                `json:"instance"`
	RestartLog     string                `json:"restart_log"`
}
