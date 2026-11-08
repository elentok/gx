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
}

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
}

// CleanupReport is the cleanup read route's result for one project.
type CleanupReport struct {
	Project   string            `json:"project"`
	Epics     []CleanupEpic     `json:"epics"`
	Worktrees []CleanupWorktree `json:"worktrees"`
	AtticRefs []string          `json:"attic_refs"`
}
