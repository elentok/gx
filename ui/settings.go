package ui

import (
	"github.com/elentok/gx/config"
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
