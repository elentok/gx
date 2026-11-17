package ops

import "time"

// Chat draft folder choices.
const (
	ChatDraftFolderSource   = "source"
	ChatDraftFolderTrunk    = "trunk"
	ChatDraftFolderWorktree = "worktree"
)

// ChatDraft is a brief and the launch settings of a chat that does not exist
// yet. Dir is the resolved folder for source and trunk, empty for worktree
// until Start. Worktree names the new worktree; WorktreePath is set once Start
// made it, StartError when a Start failed.
type ChatDraft struct {
	ID           string    `json:"id"`
	Project      string    `json:"project"`
	Name         string    `json:"name"`
	Harness      string    `json:"harness"`
	Model        string    `json:"model"`
	Effort       string    `json:"effort"`
	Brief        string    `json:"brief"`
	Folder       string    `json:"folder"`
	Dir          string    `json:"dir"`
	Worktree     string    `json:"worktree"`
	WorktreePath string    `json:"worktree_path"`
	StartError   string    `json:"start_error"`
	Revision     int       `json:"revision"`
	EditedAt     time.Time `json:"edited_at"`
	// Label is computed by the server on every read (never stored).
	Label string `json:"label"`
}

// ChatDraftCreateRequest creates a chat draft. FromSession names the session
// whose workspace and folder the draft starts from.
type ChatDraftCreateRequest struct {
	Project     string `json:"project"`
	Brief       string `json:"brief"`
	Name        string `json:"name"`
	Harness     string `json:"harness"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	Folder      string `json:"folder"`
	Worktree    string `json:"worktree"`
	FromSession string `json:"from_session"`
}

// ChatDraftReadRequest names one draft on the read and delete routes.
type ChatDraftReadRequest struct {
	ID string `json:"id"`
}

// ChatDraftSaveRequest saves the brief of a draft against the revision the caller saw.
type ChatDraftSaveRequest struct {
	ID       string `json:"id"`
	Brief    string `json:"brief"`
	Revision int    `json:"revision"`
}

// ChatDraftResult answers every chat draft route. A refusal is a result:
// Reason is a wire.Reason* or a ReasonDraft* constant. Started is the session
// id a gone draft became, when a Start record exists. Deleted: a delete
// removed the draft.
type ChatDraftResult struct {
	Refused bool       `json:"refused,omitempty"`
	Reason  string     `json:"reason,omitempty"`
	Message string     `json:"message,omitempty"`
	Deleted bool       `json:"deleted,omitempty"`
	Draft   *ChatDraft `json:"draft,omitempty"`
	Started string     `json:"started,omitempty"`
}
