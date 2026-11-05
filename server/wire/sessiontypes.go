package wire

import (
	"time"

	"github.com/elentok/gx/ralphloop/agent"
)

// SessionInfo is an interactive session on the wire: everything the Agents
// tab shows.
type SessionInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Project string `json:"project"`
	Dir     string `json:"dir"`
	Harness string `json:"harness"`
	// Ticket is the Session ticket's address.
	Ticket string `json:"ticket,omitempty"`
	// Design marks a session whose ticket is on a map epic: a gx-design run.
	Design bool `json:"design,omitempty"`
	// Queue marks a Queue agent row: a PTY-hosted ticket iteration. It has
	// no Unread or Actionable, and Holder is the agent dir the TUI opens the
	// live panel from.
	Queue  bool   `json:"queue,omitempty"`
	Holder string `json:"holder,omitempty"`
	// WaitingLabel is the Queue row's check-lock label (see checkWaitingLabel).
	WaitingLabel    string        `json:"waiting_label,omitempty"`
	Status          SessionStatus `json:"status"`
	StatusChangedAt time.Time     `json:"status_changed_at"`
	// RunStartedAt is when the current run began: the last Restore, else the
	// creation. Unlike StatusChangedAt it ignores Idle/Working flips.
	RunStartedAt time.Time `json:"run_started_at"`
	// Exit is how an Exited session ended.
	Exit string `json:"exit,omitempty"`
	// Stopped marks a session the person stopped; it is never auto-resumed.
	Stopped    bool              `json:"stopped,omitempty"`
	Unread     SessionUnread     `json:"unread"`
	Actionable SessionActionable `json:"actionable"`
	// AutoAdvance is the session's auto-advance switch; on unless the person
	// turned it off.
	AutoAdvance bool `json:"auto_advance"`
	// AutoAdvanceAt is when the running countdown sends the Continue; unset
	// with none running.
	AutoAdvanceAt *time.Time `json:"auto_advance_at,omitempty"`
	// ReportedStep is the Design step the agent says it is running, and
	// ReportedTitle the ticket it is on during Resolve tickets. Both clear when
	// the session goes idle.
	ReportedStep  string `json:"reported_step,omitempty"`
	ReportedTitle string `json:"reported_title,omitempty"`
	// Failure is set while the session is in one of the failure states.
	Failure *SessionFailure `json:"failure,omitempty"`
	// RebaseNote says why a Design open did not rebase the design branch onto
	// trunk; empty when it did.
	RebaseNote string `json:"rebase_note,omitempty"`
	// Tier, Model and Effort are what the session launched under. For a Claude
	// session Model is the one its newest reply reports, once there is a reply.
	Tier   agent.Tier `json:"tier,omitempty"`
	Model  string     `json:"model,omitempty"`
	Effort string     `json:"effort,omitempty"`
	// ModelSwitched is set when Model is not the one the launch tier chose, so
	// the person switched it by hand. Never set for a launch with no explicit model.
	ModelSwitched bool `json:"model_switched,omitempty"`
}

// SessionStatus is a Session status.
type SessionStatus struct {
	State string `json:"state"`
	// Kind and Message are what a Blocked session waits on.
	Kind       string `json:"kind,omitempty"`
	Message    string `json:"message,omitempty"`
	Background bool   `json:"background,omitempty"`
}

// Promptable reports whether the session waits for the person: Idle, or a
// finished turn that left a background task running (Working with Background).
func (st SessionStatus) Promptable() bool {
	return st.State == SessionIdle || st.State == SessionWorking && st.Background
}

// SessionUnread is a session's Unread; Resumed marks one raised by a resume.
type SessionUnread struct {
	Unread  bool `json:"unread,omitempty"`
	Resumed bool `json:"resumed,omitempty"`
}

// SessionActionable is set while Reason is; Action is its ordered prompts.
// Kind is the block's kind on one derived from Blocked, which wins over an
// agent-raised one while the block lasts. A "queue" Kind has no prompts: its
// Target is the epic whose queue the Action runs. GxRaised marks one gx raised
// itself: the person's input does not clear it.
type SessionActionable struct {
	Kind     string   `json:"kind,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Action   []string `json:"action,omitempty"`
	Target   string   `json:"target,omitempty"`
	GxRaised bool     `json:"gx_raised,omitempty"`
	// Grilling marks the Actionable of a gx-design grilling round, so clients
	// do not match its Reason text.
	Grilling bool `json:"grilling,omitempty"`
	// Derived marks an Actionable the server computed from the last assistant
	// turn of a Design session. The next idle recomputes it.
	Derived bool `json:"derived,omitempty"`
}

// SessionActionableQueue is the Kind of an Actionable whose Action is a queue
// action on Target, not prompts.
const SessionActionableQueue = "queue"

// IsQueue reports whether the Action queues Target's epic instead of sending
// prompts.
func (a SessionActionable) IsQueue() bool {
	return a.Kind == SessionActionableQueue && a.Target != ""
}

// MarkActionableRequest is the body of `gx agent actionable`: the caller's
// session, why it needs the person and its optional Action.
type MarkActionableRequest struct {
	ID     string   `json:"id"`
	Reason string   `json:"reason"`
	Action []string `json:"action,omitempty"`
}

// LinkSessionRequest is the body of `gx agent link`: the caller's session
// and its Session ticket's address.
type LinkSessionRequest struct {
	ID     string `json:"id"`
	Ticket string `json:"ticket"`
}

// ReportStepRequest is the body of `gx agent step`: the caller's session, the
// Design step it is running and, during Resolve tickets, the ticket's title.
type ReportStepRequest struct {
	ID    string `json:"id"`
	Step  string `json:"step"`
	Title string `json:"title,omitempty"`
}

// CreateSessionRequest is the body of a session create. Cols and Rows are the
// creating client's terminal panel; zero means the loop default.
type CreateSessionRequest struct {
	Project string `json:"project"`
	// Dir is where the session runs; Worktree replaces it.
	Dir string `json:"dir,omitempty"`
	// Worktree names a new worktree, made from trunk by the Worktrees tab's
	// path and branch rule, for the session to run in. gx never deletes it.
	Worktree string `json:"worktree,omitempty"`
	// Name falls back to Worktree, then the harness's Display name, then
	// `<project>-<n>`.
	Name string `json:"name,omitempty"`
	// UniqueName refuses the create when a live session already has Name.
	UniqueName bool   `json:"unique_name,omitempty"`
	Ticket     string `json:"ticket,omitempty"`
	// Prompt is typed at the session's first Idle; empty starts it Idle.
	Prompt string `json:"prompt,omitempty"`
	// Harness defaults to claude.
	Harness string `json:"harness,omitempty"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`

	// MadeWorktree marks Dir as a worktree gx made, as Worktree does.
	MadeWorktree bool `json:"made_worktree,omitempty"`
	// RebaseNote is saved in the meta as RebaseNote.
	RebaseNote string `json:"rebase_note,omitempty"`
	// Tier is the model tier; empty means worker.
	Tier agent.Tier `json:"tier,omitempty"`
}

// SessionResult is a session write's outcome: the session, or a refusal.
type SessionResult struct {
	Session *SessionInfo `json:"session,omitempty"`
	Refused bool         `json:"refused,omitempty"`
	Reason  string       `json:"reason,omitempty"`
	Message string       `json:"message,omitempty"`
	// Client is the TUI a focus request went to; empty with no TUI connected.
	Client string `json:"client,omitempty"`
	// EpicCreated and StartError report a New Design open that wrote the map
	// epic but whose session did not start: the Design exists, Refused is false.
	EpicCreated bool   `json:"epic_created,omitempty"`
	StartError  string `json:"start_error,omitempty"`
}
