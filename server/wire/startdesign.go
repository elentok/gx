package wire

// Refusal reasons of the start-design routes, beside the session ones.
const (
	ReasonStartDesignUnknownProject = "unknown-project"
	ReasonStartDesignNotBacklog        = "not-backlog"
	ReasonStartDesignNoOpenTickets  = "no-open-tickets"
	ReasonStartDesignMoveFailed     = "move-failed"
)

// StartDesignRequest is the body of POST /v1/backlog/start-design. Hint is the
// person's optional steer for the theme.
type StartDesignRequest struct {
	Project string `json:"project"`
	Backlog    string `json:"backlog"`
	Hint    string `json:"hint,omitempty"`
}

// FinishMoveRequest is the body of POST /v1/backlog/finish-move: carry on the
// unfinished move into the named charting map.
type FinishMoveRequest struct {
	Project string `json:"project"`
	Backlog    string `json:"backlog"`
	Map     string `json:"map"`
}

// FinishMoveResult answers a finish-move. A refusal is a result: Refused is
// set with Reason and Message; otherwise Address is the map's address.
type FinishMoveResult struct {
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	Address string `json:"address,omitempty"`
}
