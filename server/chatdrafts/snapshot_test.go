package chatdrafts_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elentok/gx/server/chatdrafts"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/server/workspaces"
)

func TestChatDraftSnapshot_RowsCarryNoBrief(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	if _, err := f.H.Client.Admin.CreateWorkspace(f.Ctx, "A"); err != nil {
		t.Fatal(err)
	}
	first := mustCreate(t, f, ops.ChatDraftCreateRequest{Brief: "secret prompt text", Name: "my-name"})
	second := mustCreate(t, f, ops.ChatDraftCreateRequest{Brief: "one two three"})
	if err := f.H.Server.Workspaces().SetMember(chatdrafts.MemberKey(second.ID), "A"); err != nil {
		t.Fatal(err)
	}
	snap, err := f.H.Client.Snapshot(f.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.ChatDrafts) != 2 || snap.ChatDrafts[0].ID != second.ID {
		t.Fatalf("rows = %+v, want two, newest first", snap.ChatDrafts)
	}
	rows := map[string]wire.ChatDraftRow{}
	for _, r := range snap.ChatDrafts {
		rows[r.ID] = r
	}
	if r := rows[first.ID]; r.Label != "my-name" || r.Project != "proj" || r.Workspace != workspaces.DefaultName || r.EditedAt.IsZero() {
		t.Errorf("first row = %+v", r)
	}
	if r := rows[second.ID]; r.Workspace != "A" || r.MemberKey != chatdrafts.MemberKey(second.ID) {
		t.Errorf("second row = %+v", r)
	}
	b, _ := json.Marshal(snap.ChatDrafts)
	if strings.Contains(string(b), "secret") || strings.Contains(string(b), `"brief"`) {
		t.Errorf("rows leak the brief: %s", b)
	}
}
