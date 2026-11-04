package wire

// ProjectRequest names a project; Path is what set-path points it at.
type ProjectRequest struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

// ProjectResult is the outcome of a project remove or set-path.
type ProjectResult struct {
	Name    string `json:"name,omitempty"`
	Repo    string `json:"repo,omitempty"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}
