package ui

import (
	"testing"

	"github.com/elentok/gx/config"
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
