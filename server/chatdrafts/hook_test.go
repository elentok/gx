package chatdrafts_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/chatdrafts"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire/ops"
)

func arm(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func disarm(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
}

func hookRig(t *testing.T, dir string) rigs.DesignsFixture {
	t.Helper()
	return rigs.StartDesigns(t, func(c *server.Config) { c.ForkHookDir = dir })
}

func rawCreate(t *testing.T, f rigs.DesignsFixture, req ops.ChatDraftCreateRequest) (int, string) {
	t.Helper()
	b, _ := json.Marshal(req)
	return servertest.WebPostJSON(t, f.Base+"/v1/agents/drafts/create", string(b), f.Ck, "", servertest.JSONPost(f.Base))
}

func TestForkHooks_SaveFailRefusesTheChatDraftCreateAndStoresNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := hookRig(t, dir)
	req := ops.ChatDraftCreateRequest{Project: "proj", Brief: "do it", Folder: ops.ChatDraftFolderTrunk}

	arm(t, dir, chatdrafts.HookSaveFail)
	if code, body := rawCreate(t, f, req); code != http.StatusInternalServerError {
		t.Fatalf("armed create = %d %s, want 500", code, body)
	}
	disarm(t, dir, chatdrafts.HookSaveFail)
	res := create(t, f, req)
	if res.Draft == nil {
		t.Fatalf("disarmed create = %+v, want a draft", res)
	}
}

func TestForkHooks_StartFailKeepsTheWorktreeAndTheErrorThenStartWorks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := hookRig(t, dir)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderWorktree, Worktree: "hook-wt"})

	arm(t, dir, chatdrafts.HookStartFail)
	res := post(t, f, "start", ops.ChatDraftReadRequest{ID: d.ID})
	if !res.Refused || res.Message != chatdrafts.ErrHookStartFail.Error() {
		t.Fatalf("armed start = %+v", res)
	}
	got := post(t, f, "read", ops.ChatDraftReadRequest{ID: d.ID}).Draft
	if got == nil || got.WorktreePath == "" || got.StartError != chatdrafts.ErrHookStartFail.Error() {
		t.Fatalf("draft after failed start = %+v, want the worktree path and the error", got)
	}
	if _, err := os.Stat(got.WorktreePath); err != nil {
		t.Errorf("worktree folder: %v", err)
	}

	disarm(t, dir, chatdrafts.HookStartFail)
	if res := post(t, f, "start", ops.ChatDraftReadRequest{ID: d.ID}); res.Started == "" {
		t.Fatalf("disarmed start = %+v, want a chat", res)
	}
}

func TestForkHooks_StartFailDoesNothingWithoutAWorktreeFolder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := hookRig(t, dir)
	arm(t, dir, chatdrafts.HookStartFail)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderTrunk})
	if res := post(t, f, "start", ops.ChatDraftReadRequest{ID: d.ID}); res.Started == "" {
		t.Fatalf("trunk start with start-fail armed = %+v, want a chat", res)
	}
}

func TestForkHooks_UnsetDirRefusesNothing(t *testing.T) {
	t.Parallel()
	if chatdrafts.HookArmed("", chatdrafts.HookSaveFail) || chatdrafts.HookArmed("", chatdrafts.HookStartFail) {
		t.Fatal("an empty hook dir armed a hook")
	}
	f := hookRig(t, "")
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderWorktree, Worktree: "plain-wt"})
	if res := post(t, f, "start", ops.ChatDraftReadRequest{ID: d.ID}); res.Started == "" {
		t.Fatalf("start with no hook dir = %+v, want a chat", res)
	}
}

func TestForkHooks_SaveFailFailsTheChatDraftSaveButNotTheStart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := hookRig(t, dir)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderTrunk})
	arm(t, dir, chatdrafts.HookSaveFail)

	b, _ := json.Marshal(ops.ChatDraftSaveRequest{ID: d.ID, Brief: "later", Revision: d.Revision})
	if code, body := servertest.WebPostJSON(t, f.Base+"/v1/agents/drafts/save", string(b), f.Ck, "", servertest.JSONPost(f.Base)); code != http.StatusInternalServerError {
		t.Fatalf("armed save = %d %s, want 500", code, body)
	}
	if got := post(t, f, "read", ops.ChatDraftReadRequest{ID: d.ID}).Draft; got == nil || got.Brief != d.Brief {
		t.Errorf("draft after failed save = %+v, want it unchanged", got)
	}
	res := post(t, f, "start", ops.ChatDraftStartRequest{ID: d.ID, Edit: &ops.ChatDraftEdit{Brief: "later"}})
	if res.Refused || res.Started == "" {
		t.Fatalf("start with save-fail armed = %+v, want a chat", res)
	}
}
