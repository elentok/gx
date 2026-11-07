package wire

// EventWorkspacesChanged announces that a workspace, a membership or a hold mark changed.
const EventWorkspacesChanged = "workspaces-changed"

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
// on (Name is the target workspace of a move); Order is the full new order of a reorder.
type WorkspaceRequest struct {
	Name  string   `json:"name"`
	To    string   `json:"to,omitempty"`
	Key   string   `json:"key,omitempty"`
	Order []string `json:"order,omitempty"`
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

// HoldRequest holds or unholds a Design or epic. Yes confirms that holding
// exits the Design's live sessions.
type HoldRequest struct {
	Epic string `json:"epic"`
	Yes  bool   `json:"yes,omitempty"`
}

// HoldResult is a hold or unhold: the held keys after it and the sessions it
// ended, or a refusal. A refusal for ReasonHoldLiveSessions lists the sessions
// that need Yes in Sessions.
type HoldResult struct {
	Held     []string `json:"held"`
	Sessions []string `json:"sessions,omitempty"`
	Refused  bool     `json:"refused,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Message  string   `json:"message,omitempty"`
}

// Refusal reasons of hold. The queued text is asserted by the web.
const (
	ReasonHoldQueued       = "hold-queued"
	ReasonHoldLiveSessions = "hold-live-sessions"
	ReasonHoldNotHoldable  = "hold-not-holdable"
	ReasonHoldUnknown      = "hold-unknown"
	HoldQueuedText         = "Remove it from the queue first"
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
)

// SortDefault is what a workspace reads as until a Sort is set.
const SortDefault = SortNeedsYou
