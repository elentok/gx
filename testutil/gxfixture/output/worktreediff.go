package output

// WorktreeTarget is one place a Worktree diff opens from.
type WorktreeTarget struct {
	// ID is the id of the chat on it; empty when no chat runs there.
	ID string `json:"id,omitempty"`
	// Path is the directory the diff reads.
	Path string `json:"path"`
	// Address is the ticket or Design the target belongs to.
	Address string `json:"address,omitempty"`
}

// WorktreeDiffOutput is what the worktree-diff scenario prints for its flows.
type WorktreeDiffOutput struct {
	// Chat is a plain chat in a worktree of the project repo.
	Chat WorktreeTarget `json:"chat"`
	// Ticket is a claimed ticket: its iteration worktree and Queue agent chat.
	Ticket WorktreeTarget `json:"ticket"`
	// Stacked is a claimed ticket whose worktree starts from BlockerBranch.
	Stacked       WorktreeTarget `json:"stacked"`
	BlockerBranch string         `json:"blocker_branch"`
	// Design is a Design chat in its Design worktree.
	Design WorktreeTarget `json:"design"`
	// FixAgent is a Fix agent chat; Path is the trunk checkout.
	FixAgent WorktreeTarget `json:"fix_agent"`
	// Clone is a chat in a separate clone of the project repo.
	Clone WorktreeTarget `json:"clone"`
	// Unclaimed is the address of a ticket with no worktree.
	Unclaimed string `json:"unclaimed"`
	// Capped is a chat in a worktree with one more untracked file than the
	// listing cap, and a committed tracked change.
	Capped WorktreeTarget `json:"capped"`
	// Oversize is a chat in a worktree with an untracked file over the blob cap
	// and a small untracked file.
	Oversize WorktreeTarget `json:"oversize"`
	// Clean is a chat in a worktree with no changes, HEAD at its base.
	Clean WorktreeTarget `json:"clean"`
	// PausedRun is a paused ticket whose run is still tracked, and its Queue agent chat.
	PausedRun WorktreeTarget `json:"paused_run"`
	// PausedStacked is a paused ticket with no run entry, stacked on
	// PausedBlockerBranch.
	PausedStacked       WorktreeTarget `json:"paused_stacked"`
	PausedBlockerBranch string         `json:"paused_blocker_branch"`
	// PausedFeature is a paused ticket with no run entry, based on the epic's
	// feature branch FeatureBranch.
	PausedFeature WorktreeTarget `json:"paused_feature"`
	FeatureBranch string         `json:"feature_branch"`
	// Triage is a managed Triage chat.
	Triage WorktreeTarget `json:"triage"`
}
