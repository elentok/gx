package wire

import "encoding/json"

// RepairRequest is the body of the repair verbs; each uses the fields it needs.
// Cwd and Branch are the caller's context: the server evaluates the caller
// guards on them, so no client can skip a guard by not implementing it.
type RepairRequest struct {
	// Address is the ticket's project:epic/NN; verify also takes project:epic
	// to check the whole epic.
	Address string `json:"address"`
	Cwd     string `json:"cwd,omitempty"`
	Branch  string `json:"branch,omitempty"`

	// merge
	From          string `json:"from,omitempty"`
	To            string `json:"to,omitempty"`
	IgnoreLiveTab bool   `json:"ignore_live_tab,omitempty"`
	Continue      bool   `json:"continue,omitempty"`
	Abort         bool   `json:"abort,omitempty"`

	// start-over
	Reason       string `json:"reason,omitempty"`
	Force        bool   `json:"force,omitempty"`
	DeleteBranch bool   `json:"delete_branch,omitempty"`

	// resume
	Base  string `json:"base,omitempty"`
	State string `json:"state,omitempty"`
	// Tier is a one-time tier (light, worker or strongest) for the ticket's next
	// launch; empty leaves the launch as it was.
	Tier string `json:"tier,omitempty"`
}

// RepairResult is what every repair verb returns: the verb's own result in
// Data, or a refusal carrying the repair package's stable reason code.
type RepairResult struct {
	Data    json.RawMessage `json:"data,omitempty"`
	Refused bool            `json:"refused,omitempty"`
	Reason  string          `json:"reason,omitempty"`
	Message string          `json:"message,omitempty"`

	// Via is "direct" when the CLI ran the verb itself because no server was
	// running; empty when the server served it.
	Via string `json:"via,omitempty"`
}
