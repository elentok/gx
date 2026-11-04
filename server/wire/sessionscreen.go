package wire

// SessionScreenResponse is a session's screen as plain text rows, without
// colours: the live holder's, or the one saved when the session exited.
type SessionScreenResponse struct {
	Rows []string `json:"rows"`
}
