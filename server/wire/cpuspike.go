package wire

// CPUSpikeResult is what became of a reported spike.
type CPUSpikeResult struct {
	// Address is the investigate ticket filed for it; empty when none was.
	Address string `json:"address,omitempty"`
	// Message says why no ticket was filed.
	Message string `json:"message,omitempty"`
}
