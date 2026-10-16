package config

import (
	"encoding/json"
	"strings"
)

const (
	defaultClaudeModel  = "sonnet"
	defaultClaudeEffort = "medium"
	defaultCodexModel   = "gpt-5.6-sol"
	defaultCodexEffort  = "medium"
)

// AgentConfig controls which model and effort level a coding agent runs
// under. An empty field means "inherit the agent CLI's own setting" — gx
// passes no flag for that knob.
type AgentConfig struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

// AgentsConfig holds per-agent model/effort settings, keyed by agent name,
// and the settings of interactive (Agents tab) sessions.
type AgentsConfig struct {
	Claude AgentConfig `json:"claude"`
	Codex  AgentConfig `json:"codex"`

	// LeaderKey lists the keys that start a leader sequence in the Agents tab.
	// The file accepts one key string or a list.
	LeaderKey []string `json:"leader-key"`
	// FocusSidebarKey lists the keys that leave the Agents terminal for the
	// sidebar, read like LeaderKey.
	FocusSidebarKey []string            `json:"focus-sidebar-key"`
	ResumeOnStart   bool                `json:"resume-on-start"`
	Banners         AgentsBannersConfig `json:"banners"`
	Sounds          AgentsSoundsConfig  `json:"sounds"`
	ClaudeArgs      []string            `json:"claude-args,omitempty"`
}

// AgentsBannersConfig switches interactive-session OS banners.
type AgentsBannersConfig struct {
	Enabled bool `json:"enabled"`
}

// AgentsSoundsConfig controls interactive-session sounds. Done and Attention
// are paths relative to the config dir; empty means the built-in sound.
type AgentsSoundsConfig struct {
	Enabled   bool   `json:"enabled"`
	Done      string `json:"done,omitempty"`
	Attention string `json:"attention,omitempty"`
}

// DefaultLeaderKey is the Agents tab leader key when none is configured.
const DefaultLeaderKey = "ctrl+a"

// DefaultFocusSidebarKey is the Agents tab focus-sidebar key when none is
// configured.
const DefaultFocusSidebarKey = "ctrl+q"

// DefaultAgentsConfig returns the agents defaults.
func DefaultAgentsConfig() AgentsConfig {
	return AgentsConfig{
		Claude:          AgentConfig{Model: defaultClaudeModel, Effort: defaultClaudeEffort},
		Codex:           AgentConfig{Model: defaultCodexModel, Effort: defaultCodexEffort},
		LeaderKey:       []string{DefaultLeaderKey},
		FocusSidebarKey: []string{DefaultFocusSidebarKey},
		ResumeOnStart:   true,
		Banners:         AgentsBannersConfig{Enabled: true},
		Sounds:          AgentsSoundsConfig{Enabled: true},
	}
}

// parseKeys reads a key setting as one key or a list of keys. It returns
// false for anything else, or for a blank key, so the caller keeps the default.
func parseKeys(raw json.RawMessage) ([]string, bool) {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		raw, _ = json.Marshal([]string{one})
	}
	var keys []string
	if json.Unmarshal(raw, &keys) != nil || len(keys) == 0 {
		return nil, false
	}
	for i, k := range keys {
		keys[i] = strings.TrimSpace(k)
		if keys[i] == "" {
			return nil, false
		}
	}
	return keys, true
}
