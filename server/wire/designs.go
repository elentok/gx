package wire

// Refusal reasons of the designs open route, beside the session ones.
const (
	ReasonDesignNotMapEpic  = "not-map-epic"
	ReasonDesignUnknownEpic = "unknown-epic"
	ReasonDesignEpicExists  = "epic-exists"
	// ReasonDesignBranchClash: an epic named <map>-design would share the
	// Design's branch name.
	ReasonDesignBranchClash = "design-branch-clash"
)

// DesignOpenRequest is the body of POST /v1/designs/open. Epic is a map
// epic's address ("project:epic") or one of its tickets ("project:epic/NN").
// New stops the Design's live session and starts a fresh one; State is the
// token of the live session the web saw, required on the web mux for New.
type DesignOpenRequest struct {
	Epic  string `json:"epic"`
	New   bool   `json:"new,omitempty"`
	State string `json:"state,omitempty"`
	// NewDesign starts a Design for Idea that has no map yet: Epic is then the
	// slug the map will take in Project, and no epic is created.
	NewDesign bool   `json:"new_design,omitempty"`
	Idea      string `json:"idea,omitempty"`
	Project   string `json:"project,omitempty"`
}

// DesignWorktreeRequest is the body of POST /v1/designs/worktree. Epic is a map
// epic's address ("project:epic"). ExcludeSession is the caller's own gx
// session, which does not count as another live design session in the rebase.
type DesignWorktreeRequest struct {
	Epic           string `json:"epic"`
	ExcludeSession string `json:"exclude_session,omitempty"`
}

// DesignWorktreeResult answers designs worktree. Path is the Design's worktree,
// or the trunk checkout once AfterBranch. Note says why the rebase was skipped;
// it is empty when the rebase ran. A refusal is a result, as for designs open.
type DesignWorktreeResult struct {
	Path        string `json:"path,omitempty"`
	Note        string `json:"note,omitempty"`
	AfterBranch bool   `json:"after_branch"`
	// Harness is the one a session started in the project runs on, so the
	// caller types its skill prefix.
	Harness string `json:"harness,omitempty"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// Refusal reason of the designs delete route, beside the unknown and not-map ones.
const ReasonDesignNotUncharted = "not-uncharted"

// DesignDeleteRequest is the body of POST /v1/designs/delete. Epic is the map
// epic's address ("project:epic").
type DesignDeleteRequest struct {
	Epic string `json:"epic"`
}
