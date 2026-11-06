package wire

import (
	"time"
)

// Actions the detail route offers. The server decides them; the UI renders them.
const (
	ActionAnswer         = "answer"
	ActionApprove        = "approve"
	ActionResume         = "resume"
	ActionRetry          = "retry"
	ActionStartOver      = "start-over"
	ActionMerge          = "merge"
	ActionChooseBase     = "choose-base"
	ActionReopenAndRetry = "reopen-and-retry"
	ActionStashMerge     = "stash-merge"
	// ActionResumeOnWorker ends a model-mismatch pause; the web posts a resume with tier=worker.
	ActionResumeOnWorker = "resume-on-worker"
)

// Outcomes of a pause round.
const (
	RoundOpen     = "open"
	RoundAnswered = "answered"
	RoundClosed   = "closed"
)

// ActionInfo is the text of one action's button.
type ActionInfo struct {
	Label   string `json:"label"`
	Tooltip string `json:"tooltip"`
}

// DetailOption is one choice of an options question.
type DetailOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Consequence string `json:"consequence,omitempty"`
}

// DetailRound is one pause round. The earlier rounds fill only Seq, By, Summary
// and Outcome; the latest fills the rest.
type DetailRound struct {
	Seq     int    `json:"seq"`
	By      string `json:"by"`
	Summary string `json:"summary"`
	Outcome string `json:"outcome"`

	Kind        string         `json:"kind,omitempty"`
	Text        string         `json:"text,omitempty"`
	Block       string         `json:"block,omitempty"`
	Handoff     string         `json:"handoff,omitempty"`
	Options     []DetailOption `json:"options,omitempty"`
	Recommended string         `json:"recommended,omitempty"`
	Yes         string         `json:"yes,omitempty"`
	No          string         `json:"no,omitempty"`

	// Answer is the pause.answer that ended the round.
	Answer *DetailAnswer `json:"answer,omitempty"`
	// ClosedReason is the pause.closed reason of a round no answer ended.
	ClosedReason string `json:"closed_reason,omitempty"`

	// PromptedAt is when a person last prompted the running agent of this open
	// round, and Prompt is the line the web shows for it.
	PromptedAt *time.Time `json:"prompted_at,omitempty"`
	Prompt     string     `json:"prompt,omitempty"`
}

// DetailAnswer is a person's reply to a round.
type DetailAnswer struct {
	Source string `json:"source,omitempty"`
	Choice string `json:"choice,omitempty"`
	Text   string `json:"text,omitempty"`
	Note   string `json:"note,omitempty"`
}

// DetailRecommendation is what an investigate agent left on a paused ticket: its
// diagnosis and the one verb of Actions it recommends.
type DetailRecommendation struct {
	Verb      string `json:"verb"`
	Diagnosis string `json:"diagnosis"`
}

// PermissionCard is the pending permission prompt of a blocked agent.
type PermissionCard struct {
	RequestID   string `json:"request_id"`
	Tool        string `json:"tool,omitempty"`
	Input       string `json:"input,omitempty"`
	Description string `json:"description,omitempty"`
	// Hint is set when the prompt cannot be approved from here; the card carries
	// no approve action then.
	Hint string `json:"hint,omitempty"`
}

// ChatMessage is one message of the latest round's exchange.
type ChatMessage struct {
	From string `json:"from"` // "agent", "gx", or the answer's source
	Text string `json:"text"`
}

// TicketDetail is the /v1/tickets/detail payload.
type TicketDetail struct {
	Address string `json:"address"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	// PauseReason picks the action the page labels: Answer, Choose base, Retry or Resume.
	PauseReason string `json:"pause_reason,omitempty"`
	// PauseLabel is PauseReason in the plain words the summaries and Queue show.
	PauseLabel string `json:"pause_label,omitempty"`
	// WaitingLabel is set while the claimed ticket's check run waits for the
	// check lock. A label only: Status and the pause fields do not change.
	WaitingLabel string `json:"waiting_label,omitempty"`
	// ModelRan and ModelExpected are the two models a model-mismatch pause shows.
	ModelRan      string `json:"model_ran,omitempty"`
	ModelExpected string `json:"model_expected,omitempty"`
	// LongTicketTurns and LongTicketThreshold feed the Long-running badge: the turn
	// count stored by the alert, and budget.long-ticket-turns. Both are 0 when unmarked.
	LongTicketTurns     int `json:"long_ticket_turns,omitempty"`
	LongTicketThreshold int `json:"long_ticket_threshold,omitempty"`
	// PauseText is the body's "## Paused" section; the body keeps it too, so the
	// client leaves it out of the description.
	PauseText string `json:"pause_text,omitempty"`
	Type      string `json:"type,omitempty"`
	Epic      string `json:"epic"`
	// MapEpic is set for a ticket of a map epic, which has no stats and never pauses.
	MapEpic bool `json:"map_epic"`
	// Body is the whole body markdown, comments included.
	Body    string          `json:"body,omitempty"`
	Related []RelatedTicket `json:"related"`
	Stats   *DetailStats    `json:"stats,omitempty"`
	// Answered is derived, never stored: the ticket is open again and its
	// latest question has an answer.
	Answered bool `json:"answered"`
	// Rounds holds every round before the latest, oldest first.
	Rounds []DetailRound `json:"rounds"`
	Latest *DetailRound  `json:"latest,omitempty"`
	// Permission is empty until a headless or pty agent reports a pending prompt.
	Permission *PermissionCard `json:"permission,omitempty"`
	Chat       []ChatMessage   `json:"chat"`
	// Runs is one line per run, oldest first; a map ticket has none.
	Runs []DetailRun `json:"runs"`
	// HasScreen is set when the latest run's agent ran in a pty, so
	// /v1/sessions/screen?ticket= has a screen to show.
	HasScreen bool `json:"has_screen,omitempty"`
	// State is the token an answer or resume sends back; empty without a round.
	State string `json:"state,omitempty"`
	// Bases are the competing blockers an ambiguous-base pause chooses between;
	// the resume sends the chosen one back as its base.
	Bases   []string `json:"bases,omitempty"`
	Actions []string `json:"actions"`
	// Recommendation is set while the open pause round carries one.
	Recommendation *DetailRecommendation `json:"recommendation,omitempty"`
	// ActionInfo gives each entry of Actions its button label and tooltip, from
	// the pause-to-verb table, so the page holds no verb text of its own.
	ActionInfo map[string]ActionInfo `json:"action_info,omitempty"`
	// ErrorReason, ParseError and Path are set for an Error ticket. ParseError is
	// set only when the file fails to parse, and then the payload carries no body
	// or stats.
	ErrorReason string `json:"error_reason,omitempty"`
	ParseError  string `json:"parse_error,omitempty"`
	Path        string `json:"path,omitempty"`
}

// Relations a related ticket has to the one on the page.
const (
	RelationBlocker = "blocker"
	RelationParent  = "parent"
	RelationFork    = "fork"
)

// RelatedTicket is one ticket line of the detail page's Related section.
type RelatedTicket struct {
	Relation string `json:"relation"`
	// Address is canonical for a ticket of the same project. A blocker that
	// names no ticket keeps the token as written.
	Address string `json:"address"`
	// Linkable is false for a blocker that names no ticket, which has nothing to link to.
	Linkable bool   `json:"linkable"`
	Title    string `json:"title"`
	// Status is the rendered-status word the epic rows use.
	Status string `json:"status"`
}

// DetailStats is the stat line of a loop ticket. Cost and ContextWindow come
// from the merge stamps and stay empty until the ticket merges. A map ticket
// has no stats.
type DetailStats struct {
	// Elapsed is in seconds; a claimed ticket ticks on the client from ClaimedAt.
	Elapsed       int        `json:"elapsed,omitempty"`
	ClaimedAt     time.Time  `json:"claimed_at,omitzero"`
	LaunchStep    LaunchStep `json:"launch_step,omitzero"`
	Cost          float64    `json:"cost,omitempty"`
	HadCodex      bool       `json:"had_codex,omitempty"`
	ContextWindow int        `json:"context_window,omitempty"`
}

// Outcomes of a run line.
const (
	RunMerged          = "merged"
	RunMergedNoCommits = "merged-no-commits"
	RunPaused          = "paused"
	RunStartedOver     = "started-over"
	RunRunning         = "running"
	RunAgentLost       = "agent-lost"
	// RunAgentGoneAfterRestart is a run a restarted server found dead with no
	// work to keep, and put back in the queue.
	RunAgentGoneAfterRestart = "agent-gone-after-restart"
	RunRelaunched            = "relaunched"
	RunCancelled             = "cancelled"
)

// How a run began. The run log keeps cold-start and resumed; the page labels
// them "new session" and "same session".
const (
	StartNewSession  = "new-session"
	StartSameSession = "same-session"
)

// DetailRun is one run line of the Runs section.
type DetailRun struct {
	Start time.Time `json:"start"`
	// Duration is in seconds; a running or lost run has none.
	Duration int    `json:"duration,omitempty"`
	Outcome  string `json:"outcome"`
	// PauseLabel is the reason label of a run that ended in a pause.
	PauseLabel string `json:"pause_label,omitempty"`
	StartKind  string `json:"start_kind"`
}
