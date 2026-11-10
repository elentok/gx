package apiclient

import (
	"context"

	"github.com/elentok/gx/server/wire"
)

// Workspaces returns the ordered workspaces.
func (r *Reads) Workspaces(ctx context.Context) (wire.WorkspaceList, error) {
	var l wire.WorkspaceList
	err := r.t.get(ctx, "/v1/workspaces", &l)
	return l, err
}

// CreateWorkspace adds a workspace last. A refusal is a result (Refused set).
func (a *Admin) CreateWorkspace(ctx context.Context, name string) (wire.WorkspaceResult, error) {
	return a.workspaceWrite(ctx, "/v1/workspaces/create", wire.WorkspaceRequest{Name: name})
}

// RenameWorkspace renames a workspace. A refusal is a result (Refused set).
func (a *Admin) RenameWorkspace(ctx context.Context, name, to string) (wire.WorkspaceResult, error) {
	return a.workspaceWrite(ctx, "/v1/workspaces/rename", wire.WorkspaceRequest{Name: name, To: to})
}

// DeleteWorkspace removes a workspace. A refusal is a result (Refused set).
func (a *Admin) DeleteWorkspace(ctx context.Context, name string) (wire.WorkspaceResult, error) {
	return a.workspaceWrite(ctx, "/v1/workspaces/delete", wire.WorkspaceRequest{Name: name})
}

// ReorderWorkspaces sets the order; names must list every workspace once.
func (a *Admin) ReorderWorkspaces(ctx context.Context, names []string) (wire.WorkspaceResult, error) {
	return a.workspaceWrite(ctx, "/v1/workspaces/reorder", wire.WorkspaceRequest{Order: names})
}

// MoveToWorkspace puts Designs, epics or chats in a workspace, each on its own.
// A refusal is a result.
func (a *Admin) MoveToWorkspace(ctx context.Context, workspace string, keys ...string) (wire.TargetsResult, error) {
	var res wire.TargetsResult
	err := a.t.post(ctx, "/v1/workspaces/move", wire.WorkspaceRequest{Targets: keys, Name: workspace}, &res)
	return res, err
}

// RemoveFromWorkspace sends a member back to the Default workspace.
func (a *Admin) RemoveFromWorkspace(ctx context.Context, key string) (wire.WorkspaceResult, error) {
	return a.workspaceWrite(ctx, "/v1/workspaces/remove", wire.WorkspaceRequest{Key: key})
}

// SetWorkspaceSort sets a workspace's Sort. A refusal is a result (Refused set).
func (a *Admin) SetWorkspaceSort(ctx context.Context, name, sort string) (wire.WorkspaceResult, error) {
	return a.workspaceWrite(ctx, "/v1/workspaces/sort", wire.WorkspaceRequest{Name: name, Sort: sort})
}

// HoldEpics holds Designs or epics, each on its own; yes confirms exiting a
// Design's live sessions for the whole request, dryRun writes nothing. A
// refusal is a result.
func (a *Admin) HoldEpics(ctx context.Context, targets []string, yes, dryRun bool) (wire.HoldResult, error) {
	var res wire.HoldResult
	err := a.t.post(ctx, "/v1/workspaces/hold", wire.HoldRequest{Targets: targets, Yes: yes, DryRun: dryRun}, &res)
	return res, err
}

// UnholdEpics clears the holds.
func (a *Admin) UnholdEpics(ctx context.Context, targets []string) (wire.HoldResult, error) {
	var res wire.HoldResult
	err := a.t.post(ctx, "/v1/workspaces/unhold", wire.HoldRequest{Targets: targets}, &res)
	return res, err
}

func (a *Admin) workspaceWrite(ctx context.Context, path string, req wire.WorkspaceRequest) (wire.WorkspaceResult, error) {
	var res wire.WorkspaceResult
	err := a.t.post(ctx, path, req, &res)
	return res, err
}
