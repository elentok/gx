package ui

import (
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
)

// Settings holds configuration shared across all views.
type Settings struct {
	UseNerdFontIcons bool
	ImageDiffs       bool // used by the status diff view
	InputModalBottom config.InputModalBottom
	Terminal         Terminal
	EnableNavigation bool
	DiffContextLines int               // used by the status diff view
	NameAliases      map[string]string // used by the worktrees view
	LogConfig        config.LogConfig
	ExecutionQueue   config.ExecutionQueueConfig
	Notifications    config.NotificationsConfig
	Skills           config.SkillsConfig
	Agents           config.AgentsConfig
}

// MaxConcurrentTicketsPerEpic returns the configured per-epic ticket limit.
func (s Settings) MaxConcurrentTicketsPerEpic() int {
	if s.ExecutionQueue.MaxConcurrentTicketsPerEpic < 1 {
		return config.DefaultExecutionQueueConfig().MaxConcurrentTicketsPerEpic
	}
	return s.ExecutionQueue.MaxConcurrentTicketsPerEpic
}

// MaxConcurrentEpics returns the configured process-wide epic limit.
func (s Settings) MaxConcurrentEpics() int {
	if s.ExecutionQueue.MaxConcurrentEpics < 1 {
		return config.DefaultExecutionQueueConfig().MaxConcurrentEpics
	}
	return s.ExecutionQueue.MaxConcurrentEpics
}

// ImplementSkill returns the configured ticket-implementation skill.
func (s Settings) ImplementSkill() string {
	if s.Skills.Implement == "" {
		return config.DefaultSkillsConfig().Implement
	}
	return s.Skills.Implement
}

// AgentConfig returns the configured model/effort for agent, falling back to
// gx's built-in defaults when s.Agents as a whole is the zero value (e.g. a
// Settings built without loading config, the same defensive shape
// MaxConcurrentTicketsPerEpic/ImplementSkill use). Once s.Agents is
// populated — which config.Load always produces, since its own defaults are
// non-zero — each kind's AgentConfig is returned exactly as given, including
// a deliberately empty Model or Effort (the "inherit the agent CLI's own
// setting" case; see config.AgentConfig).
func (s Settings) AgentConfig(agent ralphloop.AgentKind) config.AgentConfig {
	agents := s.Agents
	if agents == (config.AgentsConfig{}) {
		agents = config.DefaultAgentsConfig()
	}
	if agent == ralphloop.AgentCodex {
		return agents.Codex
	}
	return agents.Claude
}

// ResolvedAgents returns s.Agents with each agent kind's AgentConfig run
// through AgentConfig's zero-value fallback — the shape ralph-loop's
// RunOptions.Agents expects (see ralphloop.resolvedAgentConfig, which applies
// no fallback of its own).
func (s Settings) ResolvedAgents() config.AgentsConfig {
	return config.AgentsConfig{
		Claude: s.AgentConfig(ralphloop.AgentClaude),
		Codex:  s.AgentConfig(ralphloop.AgentCodex),
	}
}
