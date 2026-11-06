package schema

import "time"

// LaunchStamp is one launch of a ticket: when it was claimed and how long each
// of the four launch steps took, in milliseconds. gx stamps one per timed launch
// at finish; a reclaim is a second launch.
type LaunchStamp struct {
	ClaimedAt  time.Time `yaml:"claimed_at"`
	TotalMs    int64     `yaml:"total_ms"`
	WorktreeMs int64     `yaml:"worktree_ms"`
	DepsMs     int64     `yaml:"deps_ms"`
	AgentMs    int64     `yaml:"agent_ms"`
	PromptMs   int64     `yaml:"prompt_ms"`
}
