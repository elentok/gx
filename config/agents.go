package config

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

// AgentsConfig holds per-agent model/effort settings, keyed by agent name.
type AgentsConfig struct {
	Claude AgentConfig `json:"claude"`
	Codex  AgentConfig `json:"codex"`
}

// DefaultAgentsConfig returns the agents defaults.
func DefaultAgentsConfig() AgentsConfig {
	return AgentsConfig{
		Claude: AgentConfig{Model: defaultClaudeModel, Effort: defaultClaudeEffort},
		Codex:  AgentConfig{Model: defaultCodexModel, Effort: defaultCodexEffort},
	}
}
