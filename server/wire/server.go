package wire

// Handshake is the /v1/handshake payload. Shared with the API client.
type Handshake struct {
	APIVersion int    `json:"api_version"`
	Build      string `json:"build"`
	Pid        int    `json:"pid"`
	// WebURL is the base URL of the web listener; empty when it is off or failed to bind.
	WebURL string `json:"web_url,omitempty"`
	// WebError is why the web listener failed to bind; empty otherwise.
	WebError string `json:"web_error,omitempty"`
	// WebSetupPending is set until the first web password exists. It is only a flag: the
	// handshake never carries the Setup code.
	WebSetupPending bool `json:"web_setup_pending,omitempty"`
	// Migrating is set from the moment the server listens until the start-up
	// migration of every project has ended, pass or fail.
	Migrating bool `json:"migrating,omitempty"`
}

// APIVersion is bumped on any incompatible change to the /v1 wire contract.
const APIVersion = 3
