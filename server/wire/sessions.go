package wire

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
