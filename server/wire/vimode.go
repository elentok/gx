package wire

// ViModeRequest sets vi mode; a nil On toggles it.
type ViModeRequest struct {
	On *bool `json:"on,omitempty"`
}

// ViModeResult is vi mode after a write.
type ViModeResult struct {
	ViMode  bool   `json:"vi_mode"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}
