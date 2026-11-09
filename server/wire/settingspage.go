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
	// Label replaces the name derived from the key path.
	Label string `json:"label,omitempty"`
	// RiskyOffOnly: only turning the value off (or resetting it) asks to confirm.
	RiskyOffOnly bool `json:"risky_off_only,omitempty"`
	// Override is how a project row relates to its global key: "cap",
	// "follows-global", or absent for a value that replaces the global one.
	Override string `json:"override,omitempty"`
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

// SettingChange sets Key to Value, or removes Key when Remove is set. Key is
// the display key path (SettingsEntry.Key).
type SettingChange struct {
	Key    string          `json:"key"`
	Value  json.RawMessage `json:"value,omitempty"`
	Remove bool            `json:"remove,omitempty"`
}

// SettingsWriteRequest applies Changes together or not at all. Project is
// empty for the global config file.
type SettingsWriteRequest struct {
	Project string          `json:"project,omitempty"`
	Changes []SettingChange `json:"changes"`
}

// SettingsWriteResult says what a write did. A refusal is a result: it names
// the failing keys, and nothing was written.
type SettingsWriteResult struct {
	Refused bool     `json:"refused,omitempty"`
	Keys    []string `json:"keys,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	// Values holds each changed key's saved value; null for a removed key.
	Values map[string]json.RawMessage `json:"values,omitempty"`
	// RestartNeeded lists the startup-only keys that wait for a restart.
	RestartNeeded []string `json:"restart_needed,omitempty"`
	// Notices are messages to show after the save, such as the harness switch.
	Notices []string `json:"notices,omitempty"`
}
