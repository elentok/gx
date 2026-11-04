package wire

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
