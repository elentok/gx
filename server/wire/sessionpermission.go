package wire

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
