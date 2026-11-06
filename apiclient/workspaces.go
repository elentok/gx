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

func (a *Admin) workspaceWrite(ctx context.Context, path string, req wire.WorkspaceRequest) (wire.WorkspaceResult, error) {
	var res wire.WorkspaceResult
	err := a.t.post(ctx, path, req, &res)
	return res, err
}
