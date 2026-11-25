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
}
