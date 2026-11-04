package wire

// SessionRequest names the session a stop or remove acts on. State is the
// token the web sends with a stop or Restore; see SessionStateToken. Only the
// web mux checks it (see checkDesignWrite): the plain mux ignores State, and on
// the web mux an empty State never matches a token, so it is refused as stale.
type SessionRequest struct {
	ID    string `json:"id"`
	State string `json:"state,omitempty"`
}
