package ops

import "time"

// Chat draft folder choices.
const (
	ChatDraftFolderSource   = "source"
	ChatDraftFolderTrunk    = "trunk"
	ChatDraftFolderWorktree = "worktree"
)

// ReasonChatDraftStartFailed: Start hit an error that is not a refusal; the draft stays.
const ReasonChatDraftStartFailed = "start-failed"

// ChatDraft is a brief and the launch settings of a chat that does not exist
// yet. Dir is the resolved folder for source and trunk, empty for worktree
// until Start. Worktree names the new worktree; WorktreePath is set once Start
// made it, StartError when a Start failed.
type ChatDraft struct {
	ID      string `json:"id"`
	Project string `json:"project"`
	Name    string `json:"name"`
	Harness string `json:"harness"`
	Model   string `json:"model"`
	Effort  string `json:"effort"`
	Brief   string `json:"brief"`
	Folder  string `json:"folder"`
	Dir     string `json:"dir"`
	// SourceDir is the folder of the chat the draft was made from, kept so a
	// folder edit back to "source" finds it again. Empty when there is no source chat.
	SourceDir    string    `json:"source_dir"`
	Worktree     string    `json:"worktree"`
	WorktreePath string    `json:"worktree_path"`
	StartError   string    `json:"start_error"`
	Revision     int       `json:"revision"`
	EditedAt     time.Time `json:"edited_at"`
	// Label is computed by the server on every read (never stored).
	Label string `json:"label"`
}

// ChatDraftCreateRequest creates a chat draft. FromSession names the session
// whose workspace and folder the draft starts from. Workspace names the
// workspace of a draft with no FromSession; the brief may be empty.
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
	Workspace   string `json:"workspace,omitempty"`
}

// ChatDraftReadRequest names one draft on the read and delete routes.
type ChatDraftReadRequest struct {
	ID string `json:"id"`
}

// ChatDraftSaveRequest saves a draft against the revision the caller saw. The
// brief is always written; a settings field left out (nil) stays as it is.
type ChatDraftSaveRequest struct {
	ID       string  `json:"id"`
	Brief    string  `json:"brief"`
	Revision int     `json:"revision"`
	Project  *string `json:"project,omitempty"`
	Name     *string `json:"name,omitempty"`
	Harness  *string `json:"harness,omitempty"`
	Model    *string `json:"model,omitempty"`
	Effort   *string `json:"effort,omitempty"`
	Folder   *string `json:"folder,omitempty"`
	Worktree *string `json:"worktree,omitempty"`
}

// Settings reports whether the save changes anything besides the brief.
func (r ChatDraftSaveRequest) Settings() bool {
	return r.Project != nil || r.Name != nil || r.Harness != nil || r.Model != nil ||
		r.Effort != nil || r.Folder != nil || r.Worktree != nil
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
