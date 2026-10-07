package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestScratchProject_AutoCreatedAndUnremovable(t *testing.T) {
	h := servertest.Start(t)
	ctx := context.Background()

	list, err := h.Client.Projects(ctx)
	if err != nil || len(list) != 1 || list[0].Name != server.ScratchProject || list[0].VCS != config.VCSNone {
		t.Fatalf("projects = %+v, %v; want just scratch with vcs none", list, err)
	}

	workspace := server.ScratchWorkspace(h.TicketStore)
	if list[0].Repo != workspace {
		t.Errorf("scratch repo = %q, want %q", list[0].Repo, workspace)
	}
	if strings.HasPrefix(workspace, h.TicketStore+string(filepath.Separator)) {
		t.Errorf("workspace %q is inside the store %q", workspace, h.TicketStore)
	}
	if fi, err := os.Stat(workspace); err != nil || !fi.IsDir() {
		t.Errorf("workspace not created: %v", err)
	}

	res, err := h.Client.RemoveProject(ctx, server.ProjectRequest{Name: server.ScratchProject})
	if err == nil && !res.Refused {
		t.Fatalf("remove scratch = %+v, want refusal", res)
	}
	if list, _ := h.Client.Projects(ctx); len(list) != 1 {
		t.Errorf("scratch gone after refused remove: %+v", list)
	}
}
