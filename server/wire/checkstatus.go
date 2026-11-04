package wire

import (
	"github.com/elentok/gx/check"
)

// ChecksStatus is what `gx server checks` and the web "Set up checks" dialog
// read: each project's check config today and what gx would recommend.
type ChecksStatus struct {
	Projects []ProjectChecks `json:"projects"`
}

// ProjectChecks is one project's check state.
type ProjectChecks struct {
	Project string `json:"project"`
	// Missing is true when the project has neither .gx/checks.json nor a
	// checks key in project.json. A project with broken config is not missing.
	Missing     bool     `json:"missing"`
	Trunk       string   `json:"trunk"`
	SaveTargets []string `json:"save_targets"`
	// Error is set when the repo file or the checks key is invalid.
	Error string      `json:"error"`
	Kinds []KindCheck `json:"kinds"`
}

// States of a check kind.
const (
	KindStateConfigured = "configured"
	KindStateDefault    = "gx-default"
	KindStateOff        = "off"
)

// KindCheck is one kind's state and detection result.
type KindCheck struct {
	Kind string `json:"kind"`
	// State is configured, gx-default or off.
	State       string            `json:"state"`
	Default     string            `json:"default"`
	Recommended *check.Candidate  `json:"recommended,omitempty"`
	Candidates  []check.Candidate `json:"candidates"`
}
