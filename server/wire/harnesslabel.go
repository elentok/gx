package wire

// HarnessLabel is what a ticket row shows as `harness · tier · model effort`.
// A waiting ticket carries the harness it will launch under and why in
// HarnessNote; a running one carries what its run launched under.
type HarnessLabel struct {
	// Harness is "claude" or "codex"; empty for a waiting ticket that follows the default.
	Harness     string `json:"harness,omitempty"`
	Tier        string `json:"tier,omitempty"`
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
	HarnessNote string `json:"harness_note,omitempty"`
}
