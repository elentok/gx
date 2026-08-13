package ui

import (
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
)

func TestExecutionQueueConcurrencyDefaults(t *testing.T) {
	settings := Settings{}
	if got := settings.MaxConcurrentTicketsPerEpic(); got != 2 {
		t.Fatalf("MaxConcurrentTicketsPerEpic() = %d, want 2", got)
	}
	if got := settings.MaxConcurrentEpics(); got != 2 {
		t.Fatalf("MaxConcurrentEpics() = %d, want 2", got)
	}
}

func TestExecutionQueueConcurrencyUsesConfiguredValues(t *testing.T) {
	settings := Settings{ExecutionQueue: config.ExecutionQueueConfig{
		MaxConcurrentTicketsPerEpic: 3,
		MaxConcurrentEpics:          4,
	}}
	if got := settings.MaxConcurrentTicketsPerEpic(); got != 3 {
		t.Fatalf("MaxConcurrentTicketsPerEpic() = %d, want 3", got)
	}
	if got := settings.MaxConcurrentEpics(); got != 4 {
		t.Fatalf("MaxConcurrentEpics() = %d, want 4", got)
	}
}

func TestImplementSkillDefaults(t *testing.T) {
	settings := Settings{}
	if got := settings.ImplementSkill(); got != "gx-implement" {
		t.Fatalf("ImplementSkill() = %q, want gx-implement", got)
	}
}

func TestImplementSkillUsesConfiguredValue(t *testing.T) {
	settings := Settings{Skills: config.SkillsConfig{Implement: "custom-implement"}}
	if got := settings.ImplementSkill(); got != "custom-implement" {
		t.Fatalf("ImplementSkill() = %q, want custom-implement", got)
	}
}

func TestAgentConfigDefaultsOnZeroValueSettings(t *testing.T) {
	settings := Settings{}
	defaults := config.DefaultAgentsConfig()

	if got := settings.AgentConfig(ralphloop.AgentClaude); got != defaults.Claude {
		t.Fatalf("AgentConfig(claude) = %+v, want %+v", got, defaults.Claude)
	}
	if got := settings.AgentConfig(ralphloop.AgentCodex); got != defaults.Codex {
		t.Fatalf("AgentConfig(codex) = %+v, want %+v", got, defaults.Codex)
	}
}

func TestAgentConfigUsesConfiguredValues(t *testing.T) {
	settings := Settings{Agents: config.AgentsConfig{
		Claude: config.AgentConfig{Model: "opus", Effort: ""},
		Codex:  config.AgentConfig{Model: "gpt-5.6-sol", Effort: "high"},
	}}

	if got := settings.AgentConfig(ralphloop.AgentClaude); got != (config.AgentConfig{Model: "opus", Effort: ""}) {
		t.Fatalf("AgentConfig(claude) = %+v, want Model=opus Effort=\"\" (deliberately empty, not defaulted)", got)
	}
	if got := settings.AgentConfig(ralphloop.AgentCodex); got != (config.AgentConfig{Model: "gpt-5.6-sol", Effort: "high"}) {
		t.Fatalf("AgentConfig(codex) = %+v, want Model=gpt-5.6-sol Effort=high", got)
	}
}
