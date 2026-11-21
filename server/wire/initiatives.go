package wire

// InitiativeList is the Initiatives page's read model: each project with at
// least one Initiative. GET /v1/initiatives. It carries no Start action.
type InitiativeList struct {
	Projects []InitiativeProject `json:"projects"`
}

// InitiativeProject is one project's Initiatives.
type InitiativeProject struct {
	Project     string          `json:"project"`
	Initiatives []InitiativeRow `json:"initiatives"`
}

// InitiativeRow is one Initiative in the list. State is "active", "stopped" or
// "finished". Done counts gated Milestones out of Total. Current is the slug of
// the first Milestone not gated, empty when all are. StoppedAt is the number of
// the Milestone whose gate stopped the Initiative, 0 unless State is "stopped".
type InitiativeRow struct {
	Address   string `json:"address"`
	Slug      string `json:"slug"`
	State     string `json:"state"`
	Done      int    `json:"done"`
	Total     int    `json:"total"`
	Current   string `json:"current"`
	StoppedAt int    `json:"stopped_at"`
}

// InitiativePage is one Initiative's page data. GET /v1/initiatives/detail.
type InitiativePage struct {
	Address     string                 `json:"address"`
	Slug        string                 `json:"slug"`
	State       string                 `json:"state"`
	StoppedAt   int                    `json:"stopped_at"`
	Destination string                 `json:"destination"`
	Milestones  []InitiativeMilestone  `json:"milestones"`
	Assumptions []InitiativeAssumption `json:"assumptions"`
	Deferred    []InitiativeDeferred   `json:"deferred"`
	GateNotes   []InitiativeGateNote   `json:"gate_notes"`
	Unparsed    []InitiativeUnparsed   `json:"unparsed"`
}

// InitiativeAssumption is one parsed Assumption.
type InitiativeAssumption struct {
	N        int    `json:"n"`
	Text     string `json:"text"`
	TestedBy string `json:"tested_by"`
	State    string `json:"state"`
	Evidence string `json:"evidence"`
}

// InitiativeDeferred is one parsed Deferred item.
type InitiativeDeferred struct {
	Item  string `json:"item"`
	To    string `json:"to"`
	Why   string `json:"why"`
	From  string `json:"from"`
	State string `json:"state"`
}

// InitiativeGateNote is one gate note: the Milestone slug, its outcome, the
// Assumption results it recorded and its taught and placed lines.
type InitiativeGateNote struct {
	Slug        string                     `json:"slug"`
	Outcome     string                     `json:"outcome"`
	Assumptions []InitiativeGateAssumption `json:"assumptions"`
	Taught      []string                   `json:"taught"`
	Placed      []string                   `json:"placed"`
}

// InitiativeGateAssumption is one "A<n> | state | evidence" line of a gate note.
type InitiativeGateAssumption struct {
	N        int    `json:"n"`
	State    string `json:"state"`
	Evidence string `json:"evidence"`
}

// InitiativeMilestone is one Milestone. State is "sketch", "designing",
// "implementing", "merged", "gated" or "broken". A sketch or broken Milestone
// has empty DesignAddress and ImplAddress; a broken one keeps Design as the
// plain slug the Initiative names.
type InitiativeMilestone struct {
	Number        int    `json:"number"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Goal          string `json:"goal"`
	State         string `json:"state"`
	Broken        bool   `json:"broken"`
	Design        string `json:"design"`
	DesignAddress string `json:"design_address"`
	ImplAddress   string `json:"impl_address"`
}

// InitiativeUnparsed is a body line that did not parse, verbatim, with the
// section it came from.
type InitiativeUnparsed struct {
	Section string `json:"section"`
	Raw     string `json:"raw"`
}
