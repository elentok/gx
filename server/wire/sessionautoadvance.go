package wire

// AutoAdvanceRequest is the body of POST /v1/sessions/auto-advance. State is
// the web's session state token. Off, not On, so a body without it leaves the
// switch on.
type AutoAdvanceRequest struct {
	ID    string `json:"id"`
	State string `json:"state,omitempty"`
	Off   bool   `json:"off,omitempty"`
}
