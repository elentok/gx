package wire

import "encoding/json"

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
