package wire

// Handshake is the /v1/handshake payload. Shared with the API client.
type Handshake struct {
	APIVersion int    `json:"api_version"`
	Build      string `json:"build"`
	Pid        int    `json:"pid"`
	// TCPAddr is the loopback address of the opt-in TCP listener; empty when off.
	TCPAddr string `json:"tcp_addr,omitempty"`
	// WebURL is the base URL of the web listener; empty when it is off or failed to bind.
	WebURL string `json:"web_url,omitempty"`
	// WebError is why the web listener failed to bind; empty otherwise.
	WebError string `json:"web_error,omitempty"`
	// WebSetupPending is set until the first web password exists. It is only a flag: the
	// handshake never carries the Setup code.
	WebSetupPending bool `json:"web_setup_pending,omitempty"`
	// HerdrUnavailable is set while herdr isn't answering and a ticket needs it,
	// so a client can say why nothing starts. HerdrText names what waits.
	HerdrUnavailable bool   `json:"herdr_unavailable,omitempty"`
	HerdrText        string `json:"herdr_text,omitempty"`
	// Migrating is set from the moment the server listens until the start-up
	// migration of every project has ended, pass or fail.
	Migrating bool `json:"migrating,omitempty"`
}
