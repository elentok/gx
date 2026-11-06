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

// MoveToWorkspace puts a Design, epic or chat in a workspace. A refusal is a result.
func (a *Admin) MoveToWorkspace(ctx context.Context, key, workspace string) (wire.WorkspaceResult, error) {
	return a.workspaceWrite(ctx, "/v1/workspaces/move", wire.WorkspaceRequest{Key: key, Name: workspace})
}

// RemoveFromWorkspace sends a member back to the Default workspace.
func (a *Admin) RemoveFromWorkspace(ctx context.Context, key string) (wire.WorkspaceResult, error) {
	return a.workspaceWrite(ctx, "/v1/workspaces/remove", wire.WorkspaceRequest{Key: key})
}

// HoldEpic holds a Design or epic; yes confirms exiting the Design's live
// sessions. A refusal is a result (Refused set).
func (a *Admin) HoldEpic(ctx context.Context, epic string, yes bool) (wire.HoldResult, error) {
	var res wire.HoldResult
	err := a.t.post(ctx, "/v1/workspaces/hold", wire.HoldRequest{Epic: epic, Yes: yes}, &res)
	return res, err
}

// UnholdEpic clears a hold.
func (a *Admin) UnholdEpic(ctx context.Context, epic string) (wire.HoldResult, error) {
	var res wire.HoldResult
	err := a.t.post(ctx, "/v1/workspaces/unhold", wire.HoldRequest{Epic: epic}, &res)
	return res, err
}

func (a *Admin) workspaceWrite(ctx context.Context, path string, req wire.WorkspaceRequest) (wire.WorkspaceResult, error) {
	var res wire.WorkspaceResult
	err := a.t.post(ctx, path, req, &res)
	return res, err
}
