package wire

import (
	"time"

	"github.com/elentok/gx/ralphloop/agent"
)

// ModelVersion is a parsed model version.
type ModelVersion struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
}

// SessionModelRow is one model the picker offers, by full resolved ID. Latest is
// a newer model of the family; SwitchEffort is the effort a switch would use.
type SessionModelRow struct {
	ID           string        `json:"id"`
	Current      bool          `json:"current,omitempty"`
	Family       string        `json:"family,omitempty"`
	Version      *ModelVersion `json:"version,omitempty"`
	Efforts      []string      `json:"efforts"`
	Latest       string        `json:"latest,omitempty"`
	SwitchEffort string        `json:"switch_effort,omitempty"`
}

// ModelAlias maps a Claude alias to its resolved id and that id's family latest.
type ModelAlias struct {
	Alias  string `json:"alias"`
	ID     string `json:"id"`
	Latest string `json:"latest,omitempty"`
}

// SessionModelsResponse is the model list of a session's harness, in the
// harness's order. Refreshing is true while a background read is still running.
type SessionModelsResponse struct {
	Models     []SessionModelRow `json:"models"`
	Aliases    []ModelAlias      `json:"aliases"`
	Refreshing bool              `json:"refreshing"`
	// DefaultModel is the resolved id of the model a launch with no model runs
	// (Claude's "default" row, Codex's isDefault row); empty when unknown.
	DefaultModel string `json:"default_model"`
	// DefaultEffort is that model's default effort; empty when the harness does not say.
	DefaultEffort string `json:"default_effort"`
}

// SessionInfo is an interactive session on the wire: everything the Agents
// tab shows.
type SessionInfo struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	// AutoName marks a name gx derives (the terminal title), as opposed to one a
	// person typed or set with the rename verb.
	AutoName bool   `json:"auto_name,omitempty"`
	Project  string `json:"project"`
	Dir      string `json:"dir"`
	Harness  string `json:"harness"`
	Ticket   string `json:"ticket,omitempty"`
	// Design marks a session whose ticket is on a map epic: a gx-design run.
	// It is true exactly when Flow is "design".
	Design bool `json:"design,omitempty"`
	// Flow is the Flow the session runs ("design" or "triage"); empty for a
	// plain session. FlowKey is the identity it runs on: a Design's map
	// address, a Triage's project name.
	Flow    string `json:"flow,omitempty"`
	FlowKey string `json:"flow_key,omitempty"`
	// FixOf marks a Fix agent: the address of the ticket it fixes, or the bare
	// project name for a failed migration.
	FixOf string `json:"fix_of,omitempty"`
	// OffersHistory is true for a session that has a Sent messages list: a
	// Design or a plain chat.
	OffersHistory bool `json:"offers_history,omitempty"`
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
	// Stopped marks a session the person stopped; the start step never restores it, even with agents.restore-sessions-on-start on.
	Stopped    bool              `json:"stopped,omitempty"`
	Unread     SessionUnread     `json:"unread"`
	Actionable SessionActionable `json:"actionable"`
	// AutoAdvance is on unless the person turned it off; AutoAdvanceAt is when
	// the running countdown sends the Continue.
	AutoAdvance   bool       `json:"auto_advance"`
	AutoAdvanceAt *time.Time `json:"auto_advance_at,omitempty"`
	// AutoQueue is the session's auto-queue switch; off unless the person turned
	// it on. AutoQueueAt is when the running countdown ends; unset with none
	// running.
	AutoQueue   bool       `json:"auto_queue"`
	AutoQueueAt *time.Time `json:"auto_queue_at,omitempty"`
	// ReportedStep and ReportedTitle (the ticket during Resolve tickets) clear
	// when the session goes idle.
	ReportedStep  string `json:"reported_step,omitempty"`
	ReportedTitle string `json:"reported_title,omitempty"`
	// FirstPromptPending is set while gx still holds the first prompt, waiting
	// for the agent to be ready to take it.
	FirstPromptPending bool `json:"first_prompt_pending,omitempty"`
	// Notices are the non-blocking terminal issues on the session, as the
	// SessionNotice* kinds; the web UI words each one.
	Notices []string `json:"notices,omitempty"`
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
	// LatestModel is the latest model of Model's family, set only when a newer
	// one is listed. LatestTiers are the tiers of the session's harness whose
	// configured model is Model, and LatestEffort the effort a switch to
	// LatestModel would run with; both are set with LatestModel only.
	LatestModel  string `json:"latest_model,omitempty"`
	LatestFamily string `json:"latest_family,omitempty"`
	// LatestTiers holds tier names as strings: tygo prints a union element
	// type of a slice without parentheses, which types it wrongly.
	LatestTiers  []string `json:"latest_tiers,omitempty"`
	LatestEffort string   `json:"latest_effort,omitempty"`
	// SwitchingModel and SwitchingEffort are what a running switch relaunches
	// the session with. They are server memory only and clear once the new
	// agent is idle or the run ends.
	SwitchingModel  string `json:"switching_model,omitempty"`
	SwitchingEffort string `json:"switching_effort,omitempty"`
	// SwitchingBack is set while the old model starts again after a failed
	// switch; SwitchingModel is then the old model.
	SwitchingBack bool `json:"switching_back,omitempty"`
	// SwitchFailedModel and SwitchFailedOld name a failed switch once the old
	// model is idle again: the model it tried and the one the session is still
	// on. Server memory only; the next switch clears them.
	SwitchFailedModel string `json:"switch_failed_model,omitempty"`
	SwitchFailedOld   string `json:"switch_failed_old,omitempty"`
	// HasHistory is true once the chat has a completed turn.
	HasHistory bool `json:"has_history,omitempty"`
	// ContextTokens, ContextLevel and ContextCeiling are the context occupancy
	// of a Claude or Codex session or a Queue agent, from its newest reply, and
	// the smart-zone token ceiling the level is measured against; all are
	// absent while there is no reading.
	ContextTokens  *int               `json:"context_tokens,omitempty"`
	ContextLevel   agent.ContextLevel `json:"context_level,omitempty"`
	ContextCeiling int                `json:"context_ceiling,omitempty"`
	// Usage is the current conversation's context split and cumulative token
	// accounting. EstimatedCost is absent for harnesses gx cannot price.
	Usage *SessionUsage `json:"usage,omitempty"`
}

// SessionUsage is the token accounting shown beside a session's composer.
type SessionUsage struct {
	InputTokens         int      `json:"input_tokens,omitempty"`
	CacheReadTokens     int      `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens int      `json:"cache_creation_tokens,omitempty"`
	TotalTokens         int64    `json:"total_tokens"`
	EstimatedCost       *float64 `json:"estimated_cost,omitempty"`
	Compactions         int      `json:"compactions,omitempty"`
	FirstContext        int      `json:"first_context,omitempty"`
	PeakContext         int      `json:"peak_context,omitempty"`
	ContextStale        bool     `json:"context_stale,omitempty"`
	ModelContextWindow  int      `json:"model_context_window,omitempty"`
	// Scope is "conversation" for Claude including subagents, or "thread" for Codex.
	Scope          string              `json:"scope,omitempty"`
	MainTokens     *int64              `json:"main_tokens,omitempty"`
	SubagentTokens *int64              `json:"subagent_tokens,omitempty"`
	Totals         *SessionTokenTotals `json:"totals,omitempty"`
}

// SessionTokenTotals: pointers tell unreported from zero.
type SessionTokenTotals struct {
	InputTokens         *int64 `json:"input_tokens,omitempty"`
	CacheReadTokens     *int64 `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens *int64 `json:"cache_creation_tokens,omitempty"`
	OutputTokens        *int64 `json:"output_tokens,omitempty"`
	ReasoningTokens     *int64 `json:"reasoning_tokens,omitempty"`
}

// SessionStatus is a Session status.
type SessionStatus struct {
	State      string `json:"state"`
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
// itself: the person's input does not clear it. Blocking names the two gx-raised
// cards that stop a prompt from landing, so clients offer Retry without matching
// Reason text.
type SessionActionable struct {
	Blocking string   `json:"blocking,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Action   []string `json:"action,omitempty"`
	Target   string   `json:"target,omitempty"`
	GxRaised bool     `json:"gx_raised,omitempty"`
	// Images are Prompt image ids a failed send did not paste.
	Images []string `json:"images,omitempty"`
	// Grilling marks the Actionable of a gx-design grilling round, so clients
	// do not match its Reason text.
	Grilling bool `json:"grilling,omitempty"`
	// Derived marks one the server computed from a Design session's last turn.
	Derived bool `json:"derived,omitempty"`
}

// The Blocking values of a SessionActionable.
const (
	BlockingDraft    = "draft"
	BlockingStarting = "starting"
)

// The kinds of a session's non-blocking terminal issue.
const (
	// SessionNoticeUntrustedRead: gx typed while the screen read was stale after a resize.
	SessionNoticeUntrustedRead = "untrusted-read"
	// SessionNoticeNoComposerMarker: the first prompt started without the composer marker.
	SessionNoticeNoComposerMarker = "no-composer-marker"
)

// SessionActionableQueue is the Kind of an Actionable that queues Target.
const SessionActionableQueue = "queue"

// IsQueue reports whether the Action queues Target's epic instead of sending
// prompts.
func (a SessionActionable) IsQueue() bool {
	return a.Kind == SessionActionableQueue && a.Target != ""
}

// MarkActionableRequest is the body of `gx agent actionable`.
type MarkActionableRequest struct {
	ID     string   `json:"id"`
	Reason string   `json:"reason"`
	Action []string `json:"action,omitempty"`
}

// LinkSessionRequest is the body of `gx agent link`.
type LinkSessionRequest struct {
	ID     string `json:"id"`
	Ticket string `json:"ticket,omitempty"`
	// Flow and FlowKey link the session to a flow that has no ticket: "triage"
	// with the project name. Ticket must then be empty.
	Flow    string `json:"flow,omitempty"`
	FlowKey string `json:"flow_key,omitempty"`
}

// ReportStepRequest is the body of `gx agent step`.
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
	// FixOf is set by the Fix agent starters; see SessionInfo.FixOf.
	FixOf string `json:"fix_of,omitempty"`
	// Prompt is typed at the session's first Idle; empty starts it Idle.
	Prompt string `json:"prompt,omitempty"`
	// Harness defaults to claude.
	Harness string `json:"harness,omitempty"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`

	MadeWorktree bool   `json:"made_worktree,omitempty"`
	RebaseNote   string `json:"rebase_note,omitempty"`
	// Tier is the model tier; empty means worker.
	Tier agent.Tier `json:"tier,omitempty"`
}

// SessionResult is a session write's outcome: the session, or a refusal.
type SessionResult struct {
	Session *SessionInfo `json:"session,omitempty"`
	Refused bool         `json:"refused,omitempty"`
	Reason  string       `json:"reason,omitempty"`
	Message string       `json:"message,omitempty"`
	Client  string       `json:"client,omitempty"`
	// EpicCreated and StartError report a New Design open that wrote the map
	// epic but whose session did not start: the Design exists, Refused is false.
	EpicCreated bool   `json:"epic_created,omitempty"`
	StartError  string `json:"start_error,omitempty"`
	// Rows is the settled screen of a keys request that set Settle.
	Rows []string `json:"rows,omitempty"`
}
