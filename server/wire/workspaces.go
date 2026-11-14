package wire

// EventWorkspacesChanged announces that a workspace, a membership or a hold mark changed.
const EventWorkspacesChanged = "workspaces-changed"

// ProjectWorktrees is one project in the /v1/projects/worktrees payload: its
// repo root, the dirs of its other worktrees and the model each tier runs under
// for the chat harness (empty when that tier inherits the CLI's own setting).
type ProjectWorktrees struct {
	Project   string            `json:"project"`
	Root      string            `json:"root"`
	Worktrees []string          `json:"worktrees"`
	Tiers     map[string]string `json:"tiers"`
	// TierModels holds configured models by harness and tier.
	TierModels map[string]map[string]string `json:"tier_models,omitempty"`
}

// Tip is one entry of the tips page. Folders is filled by the call for tips
// that point at the project worktree folders.
type Tip struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Platform string   `json:"platform"` // GOOS value the tip applies to
	Folders  []string `json:"folders,omitempty"`
}

// Workspace is one workspace as the list and the snapshot show it.
type Workspace struct {
	Name    string `json:"name"`
	Default bool   `json:"default,omitempty"`
	Members int    `json:"members"`
	// NeedsYou counts the workspace's Needs-you cards; filled in by the server, 0 when none.
	NeedsYou int `json:"needs_you,omitempty"`
	// Sort is one of the Sort* values.
	Sort string `json:"sort,omitempty"`
}

// WorkspaceList is the workspaces in order.
type WorkspaceList struct {
	Workspaces []Workspace `json:"workspaces"`
}

// WorkspaceRequest is a workspace write: Name is the workspace acted on, To the
// new name of a rename. Key is the Design, epic or chat a move or remove acts
// on (a remove; Name is the target workspace of a move); Targets are the keys a move acts on;
// Order is the full new order of a reorder.
// Sort is the Sort* value a sort write sets on Name.
type WorkspaceRequest struct {
	Name    string   `json:"name"`
	To      string   `json:"to,omitempty"`
	Key     string   `json:"key,omitempty"`
	Targets []string `json:"targets,omitempty"`
	Order   []string `json:"order,omitempty"`
	Sort    string   `json:"sort,omitempty"`
}

// Where a session sits in the sidebar.
const (
	PlacementChat  = "chat"
	PlacementEpic  = "epic"
	PlacementOther = "other"
)

// SessionPlacement says where one session sits: a chat (with its workspace
// name), under an epic (that epic's address), or in Other.
type SessionPlacement struct {
	Session   string `json:"session"`
	Kind      string `json:"kind"`
	Workspace string `json:"workspace,omitempty"`
	Epic      string `json:"epic,omitempty"`
}

// WorkspaceResult is a workspace write: the list after it, or a refusal.
type WorkspaceResult struct {
	Workspaces []Workspace `json:"workspaces,omitempty"`
	Refused    bool        `json:"refused,omitempty"`
	Reason     string      `json:"reason,omitempty"`
	Message    string      `json:"message,omitempty"`
}

// HoldRequest holds or unholds Designs or epics. Yes confirms, for the whole
// request, that holding exits a Design's live sessions. DryRun runs every rule
// and writes nothing.
type HoldRequest struct {
	Targets []string `json:"targets"`
	Yes     bool     `json:"yes,omitempty"`
	DryRun  bool     `json:"dry_run,omitempty"`
}

// Refusal reasons of hold. The queued text is asserted by the web.
const (
	ReasonHoldQueued      = "hold-queued"
	ReasonHoldNotHoldable = "hold-not-holdable"
	ReasonHoldUnknown     = "hold-unknown"
	ReasonNotOnHold       = "not-on-hold"
	HoldQueuedText        = "Remove it from the queue first"
)

// Refusal reasons of the workspace writes.
const (
	ReasonWorkspaceBlankName    = "workspace-blank-name"
	ReasonWorkspaceNameTaken    = "workspace-name-taken"
	ReasonWorkspaceUnknown      = "workspace-unknown"
	ReasonWorkspaceDefaultFixed = "workspace-default-fixed"
	ReasonWorkspaceBadOrder     = "workspace-bad-order"
	ReasonWorkspaceBlankKey     = "workspace-blank-key"
	ReasonWorkspaceUnknownKey   = "workspace-unknown-key"
	ReasonWorkspaceFollowsEpic  = "workspace-follows-epic"
	ReasonWorkspaceNotJoinable  = "workspace-not-joinable"
	ReasonWorkspaceBadSort      = "workspace-bad-sort"
)

// Sort values of a workspace. The labels live in the web.
const (
	SortNeedsYou = "needs-you"
	SortActivity = "activity"
	SortName     = "name"
	SortProject  = "project"
	SortStatus   = "status"
)

// SortDefault is what a workspace reads as until a Sort is set.
const SortDefault = SortNeedsYou
