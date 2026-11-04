package wire

import "encoding/json"

// SaveChecksRequest saves one project's check config. Checks has the shape of
// .gx/checks.json.
type SaveChecksRequest struct {
	Project string          `json:"project"`
	Target  string          `json:"target"`
	Checks  json.RawMessage `json:"checks"`
}

// SaveChecksResult says what a save did. A refusal saved nothing.
type SaveChecksResult struct {
	Project string `json:"project,omitempty"`
	Target  string `json:"target,omitempty"`
	// Trunk and Commit are set for a repo save. Commit is empty when the file
	// already held this config.
	Trunk  string `json:"trunk,omitempty"`
	Commit string `json:"commit,omitempty"`
	// Path is the file written for a project.json save.
	Path    string `json:"path,omitempty"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// Refusal reasons of a check save.
const (
	ReasonInvalidChecks = "invalid-checks"
	ReasonBadTarget     = "bad-target"
	ReasonVCSNone       = "vcs-none"
	ReasonNoTrunk       = "no-trunk-checkout"
	ReasonWrongBranch   = "wrong-branch"
	ReasonDirtyChecks   = "dirty-checks"
	ReasonCommitFailed  = "commit-failed"
)
