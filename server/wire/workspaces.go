package wire

// EventWorkspacesChanged announces that a workspace, a membership or a hold mark changed.
const EventWorkspacesChanged = "workspaces-changed"

// Workspace is one workspace as the list and the snapshot show it.
type Workspace struct {
	Name    string `json:"name"`
	Default bool   `json:"default,omitempty"`
	Members int    `json:"members"`
}

// WorkspaceList is the workspaces in order.
type WorkspaceList struct {
	Workspaces []Workspace `json:"workspaces"`
}

// WorkspaceRequest is a workspace write: Name is the workspace acted on, To the
// new name of a rename.
type WorkspaceRequest struct {
	Name string `json:"name"`
	To   string `json:"to,omitempty"`
}

// WorkspaceResult is a workspace write: the list after it, or a refusal.
type WorkspaceResult struct {
	Workspaces []Workspace `json:"workspaces,omitempty"`
	Refused    bool        `json:"refused,omitempty"`
	Reason     string      `json:"reason,omitempty"`
	Message    string      `json:"message,omitempty"`
}

// Refusal reasons of the workspace writes.
const (
	ReasonWorkspaceBlankName    = "workspace-blank-name"
	ReasonWorkspaceNameTaken    = "workspace-name-taken"
	ReasonWorkspaceUnknown      = "workspace-unknown"
	ReasonWorkspaceDefaultFixed = "workspace-default-fixed"
)
