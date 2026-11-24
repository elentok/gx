package ops

// NotifyRequest is the body of POST /v1/notify/send.
type NotifyRequest struct {
	Message string `json:"message"`
}

// NotifyChannel is what one channel did with a message. Outcome is one of the
// notify.Outcome* strings: sent, off, unchecked, muted, tripped, unconfigured.
type NotifyChannel struct {
	Channel string `json:"channel"`
	Outcome string `json:"outcome"`
}

// NotifyResult lists all five channels. A channel that did not send is an
// outcome here, never an HTTP error (ADR 0028).
type NotifyResult struct {
	Channels []NotifyChannel `json:"channels"`
}
