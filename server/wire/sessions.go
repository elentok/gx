package wire

import (
	"time"
)

// EventSessionChanged announces a new or changed interactive session; its
// Address is the session id.
const EventSessionChanged = "session-changed"

// Refusal reasons of the interactive-session writes.
const (
	ReasonSessionBadRequest     = "bad-request"
	ReasonSessionUnknownHarness = "unknown-harness"
	ReasonSessionHarnessMissing = "harness-missing"
	ReasonSessionUnknown        = "unknown-session"
	ReasonSessionLive           = "session-live"
	ReasonSessionNotLive        = "session-not-live"
	ReasonSessionNotIdle        = "session-not-idle"
	ReasonSessionNoAction       = "no-action"
	ReasonSessionDirGone        = "dir-gone"
	ReasonSessionNoResume       = "no-resume"
	ReasonSessionNotDesign      = "not-design-session"
	ReasonSessionNameTaken      = "name-taken"
	ReasonSessionQueue          = "queue-session"
	ReasonSessionNotPlain       = "not-plain-session"
)

// Session states, as SessionStatus.State.
const (
	SessionStarting    = "starting"
	SessionWorking     = "working"
	SessionIdle        = "idle"
	SessionBlocked     = "blocked"
	SessionRateLimited = "rate-limited"
	SessionExited      = "exited"
)

// How an Exited session ended, as SessionInfo.Exit.
const (
	SessionExitClean   = "clean"
	SessionExitCrashed = "crashed"
)

// AutoAdvanceRequest is the body of POST /v1/sessions/auto-advance. State is
// the web's session state token. Off, not On, so a body without it leaves the
// switch on.
type AutoAdvanceRequest struct {
	ID    string `json:"id"`
	State string `json:"state,omitempty"`
	Off   bool   `json:"off,omitempty"`
}

// SessionChatMessage is one visible message of a session's conversation. ID is
// the transcript line's uuid.
type SessionChatMessage struct {
	ID   string    `json:"id"`
	Role string    `json:"role"` // transcript.ChatRoleAgent or ChatRolePerson
	Kind string    `json:"kind"` // text, prompt, command or output
	Text string    `json:"text"`
	At   time.Time `json:"at"`
	// ToolCount and LastTool describe a tools entry, which folds a run of tool
	// calls and carries no Text.
	ToolCount int    `json:"tool_count,omitempty"`
	LastTool  string `json:"last_tool,omitempty"`
	// Calls is one line per folded tool call: the tool and its command or target.
	Calls []string `json:"calls,omitempty"`
}

// SessionTranscriptResponse locates a session's conversation file. Harness
// and ConversationID are the session's own, so they are set even when Found is
// false.
type SessionTranscriptResponse struct {
	Harness        string `json:"harness"`
	ConversationID string `json:"conversation_id"`
	Path           string `json:"path"`
	Found          bool   `json:"found"`
}

// SessionChatResponse is the visible messages of a session's newest
// conversation. ConversationID changes after a /clear. Unsupported marks a
// session of a harness that has no chat; its Messages stay empty. Truncated
// says earlier messages were cut to fit maxChatMessages. Round holds the cards
// of a Grilling round the agent ended its last turn with and the person has not
// answered; Next is the run-end line of that turn; Background lists the shells,
// Monitors and subagents still running.
type SessionChatResponse struct {
	ConversationID string                  `json:"conversation_id"`
	Messages       []SessionChatMessage    `json:"messages"`
	Unsupported    bool                    `json:"unsupported,omitempty"`
	Truncated      bool                    `json:"truncated,omitempty"`
	Round          []SessionRoundCard      `json:"round" tstype:"SessionRoundCard[] | null"`
	Next           *SessionNext            `json:"next"`
	Background     []SessionBackgroundTask `json:"background"`
	// Failure is the session's, as on its SessionInfo.
	Failure *SessionFailure `json:"failure,omitempty"`
	// Unreadable marks a Codex conversation that has turns but no message gx
	// could read; CodexVersion is the Codex that wrote it.
	Unreadable   bool   `json:"unreadable,omitempty"`
	CodexVersion string `json:"codex_version,omitempty"`
	// Archived marks a Codex conversation whose rollout is a .jsonl.zst: the
	// messages stay empty until the archive route decodes it.
	Archived bool `json:"archived,omitempty"`
}

// SessionCommandEntry is one row of the command menu. Name has no leading
// "/" or "$".
type SessionCommandEntry struct {
	Name        string   `json:"name"`
	ArgHint     string   `json:"arg_hint"`
	Description string   `json:"description"`
	Source      string   `json:"source"` // built-in, skill or mcp
	Aliases     []string `json:"aliases"`
}

// SessionCommandsResponse is the menu for one session or project dir. Degraded
// means the harness did not answer and Entries are only the built-ins.
type SessionCommandsResponse struct {
	Entries  []SessionCommandEntry `json:"entries"`
	Degraded bool                  `json:"degraded"`
}

// Kinds of SessionFailure. Not named SessionFailure*: tygo would read them as
// the struct's own enum.
const (
	FailureKindCrashed   = "crashed"
	FailureKindEnded     = "ended"
	FailureKindAuth      = "auth"
	FailureKindRateLimit = "rate-limit"
	FailureKindStalled   = "stalled"
	FailureKindBlocked   = "blocked"
)

// SessionFailure is why a session needs the person, derived from its status
// and, for auth and stalled, from its transcript. It clears with its
// condition. Recrashed marks a crashed session whose Restore crashed again
// within restoreCrashWindow, so the page can stop offering Restore.
type SessionFailure struct {
	Kind string `json:"kind"`
	// Why says what a blocked session waits on: permission, dialog, question or
	// error (a harness.BlockedKind).
	Why       string `json:"why,omitempty"`
	Label     string `json:"label"`
	Text      string `json:"text,omitempty"`
	Recrashed bool   `json:"recrashed,omitempty"`
}

// ReasonFocusNotConnected refuses a focus report from a client with no open
// event stream: the report would never expire.
const ReasonFocusNotConnected = "not-connected"

// EventSessionFocus asks a TUI to show a session; its Address is the session
// id and its Client the TUI.
const EventSessionFocus = "session-focus"

// FocusReport is a TUI's terminal focus, the session it has on screen (empty
// for none) and its terminal app. Client names the TUI's event stream
// (/v1/events?client=); the report counts only while that stream is open.
type FocusReport struct {
	Client      string `json:"client"`
	Focused     bool   `json:"focused,omitempty"`
	Session     string `json:"session,omitempty"`
	TermProgram string `json:"term_program,omitempty"`
	// Web marks a web tab's report. Only the web mux sets it, never the body.
	Web bool `json:"-"`
}

// ReasonSessionReadOnly refuses input to a Queue agent: its loop owns it.
const ReasonSessionReadOnly = "session-read-only"

// ReasonKeysNotDelivered: a guarded send left the session Blocked.
const ReasonKeysNotDelivered = "not-delivered"

// SessionKeysRequest is the body of POST /v1/sessions/keys. Exactly one input
// is set: Key (a named key) or Text (typed without Enter). OnlyBlocked sends
// only while the session is Blocked, and waits for it to leave Blocked.
type SessionKeysRequest struct {
	ID          string `json:"id"`
	Key         string `json:"key,omitempty"`
	Text        string `json:"text,omitempty"`
	OnlyBlocked bool   `json:"only_blocked,omitempty"`
	// Permission ("allow" or "deny") answers the permission dialog the session
	// is Blocked on, by the answer logic the Queue uses. RequestID, when set,
	// must be the dialog on screen.
	Permission string `json:"permission,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
}

// Kinds of SessionPermissionResponse.
const (
	PermissionKindPrompt     = "prompt"
	PermissionKindPlan       = "plan"
	PermissionKindUnreadable = "unreadable"
)

// SessionPermissionResponse is the dialog a Blocked session waits on, read by
// the server so the page never parses a screen. RequestID is what a permission
// answer sends back; it is empty when Kind is unreadable.
type SessionPermissionResponse struct {
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	RequestID string `json:"request_id"`
}

// Refusal reasons of the session prompt route, beside the session ones.
const (
	ReasonPromptHarnessNotSupported = "harness-not-supported"
	ReasonPromptPrefixNotAllowed    = "prefix-not-allowed"
	ReasonPromptTextTooLong         = "text-too-long"
)

// Refusal reasons of POST /v1/sessions/images.
const (
	ReasonImageTypeNotAllowed = "image-type-not-allowed"
	ReasonImageTooLarge       = "image-too-large"
)

// SessionImageResult is the answer of POST /v1/sessions/images. ID names the
// saved Prompt image; Name is its file name in the session's images folder.
type SessionImageResult struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// SessionPromptRequest is the body of POST /v1/sessions/prompt. LastMessage is
// the id of the last chat message the page saw; the web mux refuses a send once
// the transcript has moved past it.
type SessionPromptRequest struct {
	ID          string `json:"id"`
	Text        string `json:"text"`
	LastMessage string `json:"last_message,omitempty"`
	// Continue replaces Text: the server types /clear and /gx-design <epic>.
	Continue bool `json:"continue,omitempty"`
	// Promote replaces Text with the gx-promote-to-design skill command. It is
	// for a plain session only, and excludes Continue.
	Promote bool `json:"promote,omitempty"`
}

// SessionRoundOption is one choice of an options card.
type SessionRoundOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Consequence string `json:"consequence,omitempty"`
}

// SessionRoundCard is one question of a Grilling round. Kind is yes-no,
// options or text; Note says why a card fell back to text.
type SessionRoundCard struct {
	Label       string               `json:"label"` // the header's question id without the Q: "8", or "8b"
	Title       string               `json:"title"`
	Body        string               `json:"body"`
	Kind        string               `json:"kind"`
	Yes         string               `json:"yes,omitempty"`
	No          string               `json:"no,omitempty"`
	Options     []SessionRoundOption `json:"options,omitempty"`
	Recommended string               `json:"recommended,omitempty"`
	Note        string               `json:"note,omitempty"`
}

// SessionNext is the Actionable a gx-design run-end line raises.
type SessionNext struct {
	Action SessionActionable `json:"action"`
}

// SessionBackgroundTask is a shell, Monitor or subagent still running.
type SessionBackgroundTask struct {
	Kind      string `json:"kind"` // transcript.BackgroundTaskShell, ...Monitor or ...Agent
	Label     string `json:"label"`
	StartedAt string `json:"started_at"`
}

// SessionScreenResponse is a session's screen as plain text rows, without
// colours: the live holder's, or the one saved when the session exited.
type SessionScreenResponse struct {
	Rows []string `json:"rows"`
}

// SessionRequest names the session a stop or remove acts on. State is the
// token the web sends with a stop or Restore; see SessionStateToken. Only the
// web mux checks it (see checkDesignWrite): the plain mux ignores State, and on
// the web mux an empty State never matches a token, so it is refused as stale.
type SessionRequest struct {
	ID    string `json:"id"`
	State string `json:"state,omitempty"`
}
