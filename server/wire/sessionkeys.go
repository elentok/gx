package wire

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
