package wire

// AddProjectRequest registers the project at Path. Name defaults to the repo
// directory's name; VCS is "" (git) or config.VCSNone.
type AddProjectRequest struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
	VCS  string `json:"vcs,omitempty"`
}

// AddProjectResult is the outcome of a project add. Existing is true when the
// path was already registered, in which case Name is the existing project's.
type AddProjectResult struct {
	Name     string `json:"name,omitempty"`
	Repo     string `json:"repo,omitempty"`
	Existing bool   `json:"existing,omitempty"`
	Refused  bool   `json:"refused,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Message  string `json:"message,omitempty"`
}

// Refusal reasons of a project add.
const (
	ReasonBadName      = "bad-name"
	ReasonReservedName = "reserved-name"
	ReasonNameTaken    = "name-taken"
	ReasonNotRepo      = "not-a-repo"
	ReasonBadVCS       = "bad-vcs"
)
