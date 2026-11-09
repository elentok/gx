package wire

import "encoding/json"

// SettingsReadRequest asks for the settings of one target. Project is empty
// for the global config file.
type SettingsReadRequest struct {
	Project string `json:"project,omitempty"`
}

// SettingsReadResponse is the registry entries that apply to a target, with
// their current values.
type SettingsReadResponse struct {
	// Project is nil for a global read.
	Project *SettingsProject `json:"project,omitempty"`
	Entries []SettingsEntry  `json:"entries"`
	// PendingRestart lists the startup-only keys changed since the server started.
	PendingRestart []string `json:"pending_restart,omitempty"`
}

// SettingsProject is a project's identity and the facts the page shows
// beside its settings.
type SettingsProject struct {
	Name string `json:"name"`
	Repo string `json:"repo,omitempty"`
	VCS  string `json:"vcs,omitempty"`
	// RepoChecksFile: the repo holds .gx/checks.json, which wins over the
	// project's checks.
	RepoChecksFile bool `json:"repo_checks_file"`
}

// SettingsEntry is one setting: what it is and what the target's file holds.
type SettingsEntry struct {
	// Key is the display key path; Path is its segments.
	Key         string   `json:"key"`
	Path        []string `json:"path"`
	Type        string   `json:"type"`
	Section     string   `json:"section"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Integer     bool     `json:"integer,omitempty"`
	Restart     bool     `json:"restart,omitempty"`
	Risky       bool     `json:"risky,omitempty"`
	ConfirmText string   `json:"confirm_text,omitempty"`
	ReadOnly    bool     `json:"read_only,omitempty"`
	// Set: the target's file holds the key. Absent means the default (or the
	// global value, in a project) applies.
	Set bool `json:"set"`
	// Value is the file's value; never sent for a secret.
	Value json.RawMessage `json:"value,omitempty"`
	// Default is the built-in default, when the key has one.
	Default json.RawMessage `json:"default,omitempty"`
	// GlobalKey and GlobalValue are, in a project read, the global key the
	// entry overrides or caps and that key's current effective value.
	GlobalKey   string          `json:"global_key,omitempty"`
	GlobalValue json.RawMessage `json:"global_value,omitempty"`
}
