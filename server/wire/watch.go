package wire

// ChangedRequest is the body of POST /v1/tickets/changed: a direct write's
// "address changed" ping.
type ChangedRequest struct {
	Address string `json:"address"`
}
