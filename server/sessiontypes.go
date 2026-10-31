package server

import (
	"cmp"
	"time"

	"github.com/elentok/gx/ralphloop"
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
	Queue           bool          `json:"queue,omitempty"`
	Holder          string        `json:"holder,omitempty"`
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
	Tier   ralphloop.Tier `json:"tier,omitempty"`
	Model  string         `json:"model,omitempty"`
	Effort string         `json:"effort,omitempty"`
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

	// design marks the session Design although Ticket has no map epic yet.
	design bool
	// MadeWorktree marks Dir as a worktree gx made, as Worktree does.
	MadeWorktree bool `json:"made_worktree,omitempty"`
	// RebaseNote is saved in the meta as RebaseNote.
	RebaseNote string `json:"rebase_note,omitempty"`
	// remoteControl is the --remote-control name, saved in the meta so resumes
	// keep it whatever the setting says by then.
	remoteControl string
	// Tier is the model tier; empty means worker.
	Tier ralphloop.Tier `json:"tier,omitempty"`
	// launchDesign marks a launch by a design path (Start design, New Design):
	// the tier follows the map's step, and the meta saves LaunchDesign.
	launchDesign bool
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
	// LaunchDesign is set only by a design launch path. Design alone (set by
	// `gx agent link` too) says nothing about how the session was launched.
	LaunchDesign bool `json:"launch_design,omitempty"`
	// HandoffTier is set while the worker model of a charting handoff has not
	// yet run well. A Resume of a crash resolves it again from the config, so a
	// fixed slug takes effect; any other Resume keeps the launch's model.
	HandoffTier bool `json:"handoff_tier,omitempty"`
	// RemoteControl is the name a Claude fix-agent session was started with
	// --remote-control under; empty means the flag was never applied.
	RemoteControl string `json:"remote_control,omitempty"`
	// Tier, Model and Effort are what the session launched under, saved so a
	// resume keeps them whatever the settings say by then. A session from before
	// tiers has none, and its resume passes no model or effort flag.
	Tier   ralphloop.Tier `json:"tier,omitempty"`
	Model  string         `json:"model,omitempty"`
	Effort string         `json:"effort,omitempty"`
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
	// AutoSends counts the auto-advance sends since the person last gave input.
	// A restart keeps it: a restart never sends, so only the next run end reads it.
	AutoSends int `json:"auto_sends,omitempty"`
	// ReportedStep and ReportedTitle are the agent's own report of its Design
	// step; the next idle clears them.
	ReportedStep  string `json:"reported_step,omitempty"`
	ReportedTitle string `json:"reported_title,omitempty"`
	// RestoredAt is when the person last Restored the session; RestoreCrashed
	// marks that run crashing within restoreCrashWindow.
	RestoredAt     time.Time `json:"restored_at,omitzero"`
	RestoreCrashed bool      `json:"restore_crashed,omitempty"`
	// RebaseNote is why the design branch was not rebased at open, if it wasn't.
	RebaseNote string `json:"rebase_note,omitempty"`
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
		ReportedStep:    m.ReportedStep,
		ReportedTitle:   m.ReportedTitle,
		RebaseNote:      m.RebaseNote,
		Tier:            m.Tier,
		Model:           m.Model,
		Effort:          m.Effort,
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
