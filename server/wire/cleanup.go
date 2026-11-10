package wire

// EventCleanupSizeChanged is emitted when a worktree's disk size lands; the
// event's address is the project.
const EventCleanupSizeChanged = "cleanup-size-changed"

// CleanupEpic is one epic's classification in the cleanup scan.
type CleanupEpic struct {
	Name                string `json:"name"`
	AllDone             bool   `json:"all_done"`
	MergedToTrunk       bool   `json:"merged_to_main"`
	HasCodeReviewTicket bool   `json:"has_code_review_ticket"`
	CodeReviewDone      bool   `json:"code_review_done"`
	// Action is the one thing the page offers for the epic: CleanupActionArchive,
	// CleanupActionAgent, or empty for none.
	Action string `json:"action,omitempty"`
}

// The per-row cleanup actions the server decides. The page only buckets by them.
const (
	CleanupActionMergedRemove   = "merged-remove"
	CleanupActionMergedDelete   = "merged-delete"
	CleanupActionUnmergedRemove = "unmerged-remove"
	CleanupActionArchive        = "archive"
	// CleanupActionAgent marks a row no button can decide, so the cleanup agent does.
	CleanupActionAgent = "agent"
)

// CleanupWorktree is one branch or detached worktree in the cleanup scan.
// SizeBytes is absent until the background size read for Path has landed.
type CleanupWorktree struct {
	Branch         string  `json:"branch"`
	Path           string  `json:"path,omitempty"`
	Kind           string  `json:"kind"` // "iteration", "feature", "other", "detached"
	Active         bool    `json:"active"`
	Dirty          bool    `json:"dirty,omitempty"`
	Head           string  `json:"head,omitempty"`
	Epic           string  `json:"epic,omitempty"`
	TicketID       string  `json:"ticket_id,omitempty"`
	TicketDone     bool    `json:"ticket_done,omitempty"`
	Merged         bool    `json:"merged,omitempty"`
	MergedToTrunk  bool    `json:"merged_to_main,omitempty"`
	DeleteMode     string  `json:"delete_mode,omitempty"`
	Recommendation string  `json:"recommendation,omitempty"`
	SizeBytes      *uint64 `json:"size_bytes,omitempty"`
	// Action is one of the CleanupAction values, or empty when the row has no
	// button (detached, live session, active and unmerged).
	Action string `json:"action,omitempty"`
}

// CleanupReport is the cleanup read route's result for one project.
type CleanupReport struct {
	Project   string            `json:"project"`
	Epics     []CleanupEpic     `json:"epics"`
	Worktrees []CleanupWorktree `json:"worktrees"`
	AtticRefs []string          `json:"attic_refs"`
}

// CleanupWriteRequest names the branch a cleanup write acts on. Remove takes a
// branch that has a worktree, Delete one that has none. Archive takes an Epic.
type CleanupWriteRequest struct {
	Project string `json:"project"`
	Branch  string `json:"branch,omitempty"`
	Epic    string `json:"epic,omitempty"`
	// Yes confirms an archive that stops the epic's live Sessions.
	Yes bool `json:"yes,omitempty"`
}

// Reasons a cleanup write refuses.
const (
	ReasonCleanupUnknown     = "cleanup-unknown"
	ReasonCleanupNotMerged   = "cleanup-not-merged"
	ReasonCleanupHasWorktree = "cleanup-has-worktree"
	ReasonCleanupNoWorktree  = "cleanup-no-worktree"
	ReasonCleanupGitRefused  = "cleanup-git-refused"
	ReasonCleanupDirty       = "cleanup-dirty"
	ReasonCleanupLive        = "cleanup-live-session"
	ReasonCleanupQueued      = "cleanup-queued"
	ReasonCleanupNotDone     = "cleanup-not-done"
)

// ReasonCleanupNothingForAgent is the refusal when no row belongs to the agent
// group.
const ReasonCleanupNothingForAgent = "cleanup-nothing-for-agent"

// CleanupWriteResult is a cleanup write's answer. A refusal is a result, not
// an error; Message then holds the reason, git's own when git refused.
type CleanupWriteResult struct {
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	// WorktreeRemoved is set when the worktree went but the branch stayed.
	WorktreeRemoved bool `json:"worktree_removed,omitempty"`
}

// Refusal and Refuse let the CLI treat the write result as a Refusable.
func (r CleanupWriteResult) Refusal() (bool, string, string) { return r.Refused, r.Reason, r.Message }
func (CleanupWriteResult) Refuse(reason, message string) CleanupWriteResult {
	return CleanupWriteResult{Refused: true, Reason: reason, Message: message}
}
