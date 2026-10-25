package server

import (
	"cmp"
	"time"
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
	Design          bool          `json:"design,omitempty"`
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
	// Failure is set while the session is in one of the failure states.
	Failure *SessionFailure `json:"failure,omitempty"`
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
	Name   string `json:"name,omitempty"`
	Ticket string `json:"ticket,omitempty"`
	// Prompt is typed at the session's first Idle; empty starts it Idle.
	Prompt string `json:"prompt,omitempty"`
	// Harness defaults to claude.
	Harness string `json:"harness,omitempty"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`

	// design marks the session Design although Ticket has no map epic yet.
	design bool
}

// SessionResult is a session write's outcome: the session, or a refusal.
type SessionResult struct {
	Session *SessionInfo `json:"session,omitempty"`
	Refused bool         `json:"refused,omitempty"`
	Reason  string       `json:"reason,omitempty"`
	Message string       `json:"message,omitempty"`
	// Client is the TUI a focus request went to; empty with no TUI connected.
	Client string `json:"client,omitempty"`
}

// sessionMeta is what a session's state dir records about it.
type sessionMeta struct {
	ID      string `json:"id"`
	Harness string `json:"harness"`
	Project string `json:"project"`
	Dir     string `json:"dir"`
	// MadeWorktree marks a Dir gx made for this session: only there is the
	// trust dialog answered for the person.
	MadeWorktree bool   `json:"made_worktree,omitempty"`
	Name         string `json:"name,omitempty"`
	// AutoName marks a fallback name the Display name replaces; a typed name
	// is never replaced.
	AutoName bool   `json:"auto_name,omitempty"`
	Ticket   string `json:"ticket,omitempty"`
	Design   bool   `json:"design,omitempty"`
	// FirstPrompt waits for the ready report or the first Idle; it is cleared
	// once typed or on exit.
	FirstPrompt string `json:"first_prompt,omitempty"`
	// ConversationID is the newest one the harness reported; a resume
	// continues it.
	ConversationID string    `json:"conversation_id"`
	CreatedAt      time.Time `json:"created_at"`
	// Run counts the session's resumes. A watch records only for the run it
	// was started for, so a stopped holder's late stream end cannot exit the
	// resumed one.
	Run int `json:"run,omitempty"`
	// Cols and Rows are the last size the session's holder started at.
	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`
	// Status, StatusChangedAt and Exit are the last seen Session status; an
	// empty State is Starting since CreatedAt.
	Status          SessionStatus `json:"status"`
	StatusChangedAt time.Time     `json:"status_changed_at,omitzero"`
	Exit            string        `json:"exit,omitempty"`
	Stopped         bool          `json:"stopped,omitempty"`
	Unread          SessionUnread `json:"unread,omitzero"`
	// Actionable is the agent-raised one, which the person's next input clears,
	// or a gx-raised one, which it spares. Exit clears either.
	Actionable SessionActionable `json:"actionable,omitzero"`
	// AutoAdvanceOff is the inverse of the switch, so a meta written before the
	// switch existed reads as on.
	AutoAdvanceOff bool `json:"auto_advance_off,omitempty"`
	// RestoredAt is when the person last Restored the session; RestoreCrashed
	// marks that run crashing within restoreCrashWindow.
	RestoredAt     time.Time `json:"restored_at,omitzero"`
	RestoreCrashed bool      `json:"restore_crashed,omitempty"`
}

func (m sessionMeta) info() SessionInfo {
	info := SessionInfo{
		ID:              m.ID,
		Name:            m.Name,
		Project:         m.Project,
		Dir:             m.Dir,
		Harness:         m.Harness,
		Ticket:          m.Ticket,
		Design:          m.Design,
		Status:          m.Status,
		StatusChangedAt: m.StatusChangedAt,
		RunStartedAt:    cmp.Or(m.RestoredAt, m.CreatedAt),
		Exit:            m.Exit,
		Stopped:         m.Stopped,
		Unread:          m.Unread,
		Actionable:      m.Actionable,
		AutoAdvance:     !m.AutoAdvanceOff,
	}
	if st := m.Status; st.State == SessionBlocked {
		info.Actionable = SessionActionable{Kind: st.Kind, Reason: cmp.Or(st.Message, st.Kind, SessionBlocked)}
	}
	if info.Status.State == "" {
		info.Status.State = SessionStarting
	}
	if info.StatusChangedAt.IsZero() {
		info.StatusChangedAt = m.CreatedAt
	}
	return info
}
