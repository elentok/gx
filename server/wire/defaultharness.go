package wire

// ProjectDefaultHarness is one project's override; Harness is empty when the
// project follows the global default.
type ProjectDefaultHarness struct {
	Project string `json:"project"`
	Harness string `json:"harness"`
}

// DefaultHarnessStatus is the Default harness the Settings page shows.
type DefaultHarnessStatus struct {
	Default  string                  `json:"default"`
	Runner   string                  `json:"runner"`
	Projects []ProjectDefaultHarness `json:"projects"`
}

// SetDefaultHarnessRequest writes the Default harness. An empty Harness with
// scope project removes the override.
type SetDefaultHarnessRequest struct {
	Scope   string `json:"scope"`
	Project string `json:"project,omitempty"`
	Harness string `json:"harness"`
}

// SetDefaultHarnessResult says what a write did. A refusal wrote nothing.
type SetDefaultHarnessResult struct {
	Scope   string `json:"scope,omitempty"`
	Project string `json:"project,omitempty"`
	// Harness is the Default harness in effect for the scope after the write.
	Harness string `json:"harness,omitempty"`
	Changed bool   `json:"changed"`
	// Waiting and Running are the notice's counts; Notice is its text, empty
	// when nothing changed.
	Waiting int    `json:"waiting"`
	Running int    `json:"running"`
	Notice  string `json:"notice,omitempty"`
	Path    string `json:"path,omitempty"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}
