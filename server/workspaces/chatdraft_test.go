package workspaces_test

import (
	"testing"

	"github.com/elentok/gx/server/chatdrafts"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

func seedChatDraft(t *testing.T, stateDir string) string {
	t.Helper()
	id, err := chatdrafts.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := chatdrafts.New(stateDir).Create(&ops.ChatDraft{ID: id, Project: "proj", Brief: "x", Folder: ops.ChatDraftFolderTrunk}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestWorkspacesRoutes_MoveChatDraft(t *testing.T) {
	f, ctx := startMembership(t)
	key := chatdrafts.MemberKey(seedChatDraft(t, f.H.StateDir))
	res, err := f.H.Client.Admin.MoveToWorkspace(ctx, "Bugs", key, chatdrafts.MemberKey("ffffffffffffffff"))
	if err != nil || len(res.Results) != 2 {
		t.Fatalf("move = %+v, %v", res, err)
	}
	if r := res.Results[0]; r.Status != wire.TargetDone {
		t.Errorf("existing chat draft = %+v, want done", r)
	}
	if r := res.Results[1]; r.Status != wire.TargetRefused || r.Reason != wire.ReasonWorkspaceUnknownKey {
		t.Errorf("unknown chat draft = %+v, want unknown-key", r)
	}
	if got := f.H.Server.Workspaces().Workspace(key); got != "Bugs" {
		t.Errorf("chat draft workspace = %q, want Bugs", got)
	}
}
