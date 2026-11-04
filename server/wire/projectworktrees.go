package wire

// ProjectWorktrees is one project in the /v1/projects/worktrees payload: its
// repo root, the dirs of its other worktrees and the model each tier runs under
// for the chat harness (empty when that tier inherits the CLI's own setting).
type ProjectWorktrees struct {
	Project   string            `json:"project"`
	Root      string            `json:"root"`
	Worktrees []string          `json:"worktrees"`
	Tiers     map[string]string `json:"tiers"`
}
