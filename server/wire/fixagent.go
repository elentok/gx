package wire

// Refusal reasons of the fix-agent route, beside the session ones.
const (
	ReasonFixUnknownTicket = "unknown-ticket"
	ReasonFixNotRepair     = "not-paused-for-repair"
	ReasonFixInvestigating = "investigating"
	ReasonFixNoFailure     = "no-migration-failure"
)

// FixAgentRequest is the body of POST /v1/tickets/fix-agent. State is the
// token of the row paused for repair the web saw, required on the web mux.
// Project instead of Address asks for an agent fixing that project's saved
// failed migration.
type FixAgentRequest struct {
	Address string `json:"address,omitempty"`
	Project string `json:"project,omitempty"`
	State   string `json:"state,omitempty"`
}

// FixAgentResult is the session result plus whether remote control was applied.
type FixAgentResult struct {
	SessionResult `tstype:",extends"`
	RemoteControl bool `json:"remote_control"`
}
