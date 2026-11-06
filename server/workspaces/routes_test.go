package workspaces_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/workspaces"
)

func names(l []wire.Workspace) []string {
	out := make([]string, len(l))
	for i, w := range l {
		out[i] = w.Name
	}
	return out
}

func TestWorkspacesRoutes_CreateRenameDeleteEachPublishOneEvent(t *testing.T) {
	h := servertest.Start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	l, err := h.Client.Reads.Workspaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{workspaces.DefaultName}; !slices.Equal(names(l.Workspaces), want) || !l.Workspaces[0].Default {
		t.Fatalf("initial list = %+v, want only the Default workspace", l.Workspaces)
	}

	res, err := h.Client.Admin.CreateWorkspace(ctx, "Bugs")
	if err != nil || res.Refused {
		t.Fatalf("create = %+v, %v", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventWorkspacesChanged, "")
	if want := []string{workspaces.DefaultName, "Bugs"}; !slices.Equal(names(res.Workspaces), want) {
		t.Errorf("after create = %v, want %v", names(res.Workspaces), want)
	}

	res, err = h.Client.Admin.RenameWorkspace(ctx, "Bugs", "Fixes")
	if err != nil || res.Refused {
		t.Fatalf("rename = %+v, %v", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventWorkspacesChanged, "")
	if want := []string{workspaces.DefaultName, "Fixes"}; !slices.Equal(names(res.Workspaces), want) {
		t.Errorf("after rename = %v, want %v", names(res.Workspaces), want)
	}

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{workspaces.DefaultName, "Fixes"}; !slices.Equal(names(snap.Workspaces), want) {
		t.Errorf("snapshot workspaces = %v, want %v", names(snap.Workspaces), want)
	}

	res, err = h.Client.Admin.DeleteWorkspace(ctx, "Fixes")
	if err != nil || res.Refused {
		t.Fatalf("delete = %+v, %v", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, wire.EventWorkspacesChanged, "")
	if want := []string{workspaces.DefaultName}; !slices.Equal(names(res.Workspaces), want) {
		t.Errorf("after delete = %v, want %v", names(res.Workspaces), want)
	}
}

func TestWorkspacesRoutes_Refusals(t *testing.T) {
	h := servertest.Start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := h.Client.Admin.CreateWorkspace(ctx, "Bugs"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		do     func() (wire.WorkspaceResult, error)
		reason string
	}{
		{"clash ignores case", func() (wire.WorkspaceResult, error) { return h.Client.Admin.CreateWorkspace(ctx, "BUGS") }, wire.ReasonWorkspaceNameTaken},
		{"blank name", func() (wire.WorkspaceResult, error) { return h.Client.Admin.CreateWorkspace(ctx, "  ") }, wire.ReasonWorkspaceBlankName},
		{"delete Default", func() (wire.WorkspaceResult, error) {
			return h.Client.Admin.DeleteWorkspace(ctx, workspaces.DefaultName)
		}, wire.ReasonWorkspaceDefaultFixed},
		{"rename unknown", func() (wire.WorkspaceResult, error) { return h.Client.Admin.RenameWorkspace(ctx, "Nope", "X") }, wire.ReasonWorkspaceUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tt.do()
			if err != nil {
				t.Fatal(err)
			}
			if !res.Refused || res.Reason != tt.reason || res.Message == "" {
				t.Errorf("result = %+v, want refused with %q and a message", res, tt.reason)
			}
		})
	}
}
