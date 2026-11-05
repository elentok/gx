package sessions

import (
	"cmp"
	"time"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/sessions/convrecord"
	"github.com/elentok/gx/server/wire"
)

// sessionCtx is what the server's own callers set on a session create that the
// body does not carry. It travels beside the CreateSessionRequest, never
// inside it.
type SessionCtx struct {
	// design marks the session Design although Ticket has no map epic yet.
	design bool
	// RemoteControl is the --remote-control name, saved in the meta so resumes
	// keep it whatever the setting says by then.
	RemoteControl string
	// LaunchDesign marks a launch by a design path (Start design, New Design):
	// the tier follows the map's step, and the meta saves LaunchDesign.
	LaunchDesign bool
}

// sessionMeta is what a session's state dir records about it.
type Meta struct {
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
	Tier   agent.Tier `json:"tier,omitempty"`
	Model  string     `json:"model,omitempty"`
	Effort string     `json:"effort,omitempty"`
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
	Status          wire.SessionStatus `json:"status"`
	StatusChangedAt time.Time          `json:"status_changed_at,omitzero"`
	Exit            string             `json:"exit,omitempty"`
	Stopped         bool               `json:"stopped,omitempty"`
	Unread          wire.SessionUnread `json:"unread,omitzero"`
	// Actionable is the agent-raised one, which the person's next input clears,
	// or a gx-raised one, which it spares. Exit clears either.
	Actionable wire.SessionActionable `json:"actionable,omitzero"`
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

// CanAutoAdvance is the one rule for whether the session's raised Actionable
// may be sent by the server: the switch is on, it is a derived Continue (not a
// grilling round, not agent- or gx-raised), the session waits for the person,
// whatever its harness. Call sites add their own conditions: the
// detector needs a live turn and the cap, the switch needs no countdown running.
func (m Meta) ConvSession() convrecord.Session {
	return convrecord.Session{ID: m.ID, Dir: m.Dir, Harness: m.Harness, Ticket: m.Ticket, ConversationID: m.ConversationID, Design: m.Design}
}

func (m Meta) CanAutoAdvance() bool {
	a := m.Actionable
	return !m.AutoAdvanceOff && a.Derived && !a.Grilling && len(a.Action) > 0 &&
		m.Status.Promptable()
}

// ChatMeta is the part of the meta the chat reads.
func (m Meta) ChatMeta() chat.Meta {
	return chat.Meta{
		ID: m.ID, Harness: m.Harness, Project: m.Project, Dir: m.Dir, Ticket: m.Ticket,
		ConversationID: m.ConversationID, Model: m.Model, Effort: m.Effort,
		Status: m.Status, Exit: m.Exit, RestoreCrashed: m.RestoreCrashed,
	}
}

func (m Meta) Info() wire.SessionInfo {
	info := wire.SessionInfo{
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
	if st := m.Status; st.State == wire.SessionBlocked {
		info.Actionable = wire.SessionActionable{Kind: st.Kind, Reason: cmp.Or(st.Message, st.Kind, wire.SessionBlocked)}
	}
	if info.Status.State == "" {
		info.Status.State = wire.SessionStarting
	}
	if info.StatusChangedAt.IsZero() {
		info.StatusChangedAt = m.CreatedAt
	}
	return info
}
