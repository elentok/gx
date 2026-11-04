package wire

// AFKRequest sets AFK mode; a nil On toggles it.
type AFKRequest struct {
	On *bool `json:"on,omitempty"`
}

// AFKResult is AFK mode after a write.
type AFKResult struct {
	AFK     bool   `json:"afk"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}
