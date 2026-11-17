package chatdrafts_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/chatdrafts"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/server/workspaces"
)

func post(t *testing.T, f rigs.DesignsFixture, path string, body any) ops.ChatDraftResult {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	code, out := servertest.WebPostJSON(t, f.Base+"/v1/agents/drafts/"+path, string(b), f.Ck, "", servertest.JSONPost(f.Base))
	if code != http.StatusOK {
		t.Fatalf("POST %s = %d %s", path, code, out)
	}
	var res ops.ChatDraftResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("POST %s body %s: %v", path, out, err)
	}
	return res
}

func create(t *testing.T, f rigs.DesignsFixture, req ops.ChatDraftCreateRequest) ops.ChatDraftResult {
	t.Helper()
	if req.Project == "" {
		req.Project = "proj"
	}
	if req.Brief == "" {
		req.Brief = "do the thing"
	}
	if req.Folder == "" {
		req.Folder = ops.ChatDraftFolderTrunk
	}
	return post(t, f, "create", req)
}

func mustCreate(t *testing.T, f rigs.DesignsFixture, req ops.ChatDraftCreateRequest) *ops.ChatDraft {
	t.Helper()
	res := create(t, f, req)
	if res.Refused || res.Draft == nil {
		t.Fatalf("create = %+v", res)
	}
	return res.Draft
}

func workspaceOf(f rigs.DesignsFixture, id string) string {
	return f.H.Server.Workspaces().Workspace(chatdrafts.MemberKey(id))
}

// plainChat makes a plain chat in workspace ws and returns it with its dir.
func plainChat(t *testing.T, f rigs.DesignsFixture, ws string) (id, dir string) {
	t.Helper()
	if ws != "" {
		if _, err := f.H.Client.Admin.CreateWorkspace(f.Ctx, ws); err != nil {
			t.Fatal(err)
		}
	}
	dir = t.TempDir()
	res, err := f.H.Client.Sessions.Create(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: dir})
	if err != nil || res.Session == nil {
		t.Fatalf("create chat = %+v, %v", res, err)
	}
	if ws != "" {
		if mv, err := f.H.Client.Admin.MoveToWorkspace(f.Ctx, ws, res.Session.ID); err != nil || mv.Results[0].Refused() {
			t.Fatalf("move chat = %+v, %v", mv, err)
		}
	}
	return res.Session.ID, res.Session.Dir
}

func designSession(t *testing.T, f rigs.DesignsFixture, ws string) string {
	t.Helper()
	if _, err := f.H.Client.Admin.CreateWorkspace(f.Ctx, ws); err != nil {
		t.Fatal(err)
	}
	res := f.Open(t, "proj:map", false)
	if res.Session == nil {
		t.Fatalf("open design = %+v", res)
	}
	if mv, err := f.H.Client.Admin.MoveToWorkspace(f.Ctx, ws, "proj:map"); err != nil || mv.Results[0].Refused() {
		t.Fatalf("move design = %+v, %v", mv, err)
	}
	return res.Session.ID
}

// draftEvents runs act and counts the chat-drafts-changed events it caused.
func draftEvents(t *testing.T, f rigs.DesignsFixture, act func()) int {
	t.Helper()
	sentinel := func() {
		name := fmt.Sprintf("sentinel-%d", time.Now().UnixNano())
		if _, err := f.H.Client.Admin.CreateWorkspace(f.Ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	sentinel()
	servertest.WaitForEvent(f.Ctx, t, f.Evs, wire.EventWorkspacesChanged, "")
	act()
	sentinel()
	n := 0
	for {
		select {
		case ev := <-f.Evs:
			switch ev.Type {
			case wire.EventChatDraftsChanged:
				n++
			case wire.EventWorkspacesChanged:
				return n
			}
		case <-f.Ctx.Done():
			t.Fatal(f.Ctx.Err())
		}
	}
}

func TestChatDraftCreate_StoresTheSettingsAndReturnsThem(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Name: "My chat", Brief: "  look at it  ", Model: "opus", Effort: "high"})
	if d.ID == "" || d.Project != "proj" || d.Name != "My chat" || d.Brief != "look at it" || d.Model != "opus" ||
		d.Effort != "high" || d.Harness == "" || d.Folder != ops.ChatDraftFolderTrunk || d.Revision != 1 || d.Label != "My chat" {
		t.Errorf("draft = %+v", d)
	}
	got := post(t, f, "read", ops.ChatDraftReadRequest{ID: d.ID})
	if got.Draft == nil || got.Draft.Brief != "look at it" {
		t.Errorf("read = %+v", got)
	}
}

func TestChatDraftCreate_PlainChatSourceJoinsItsWorkspace(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	chat, _ := plainChat(t, f, "Bugs")
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{FromSession: chat})
	if got := workspaceOf(f, d.ID); got != "Bugs" {
		t.Errorf("workspace = %q, want Bugs", got)
	}
}

func TestChatDraftCreate_DesignSourceJoinsTheDesignsWorkspace(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	design := designSession(t, f, "Maps")
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{FromSession: design})
	if got := workspaceOf(f, d.ID); got != "Maps" {
		t.Errorf("workspace = %q, want Maps", got)
	}
}

func TestChatDraftCreate_NoOrUnknownSourceLandsInDefault(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	for _, from := range []string{"", "no-such-session"} {
		d := mustCreate(t, f, ops.ChatDraftCreateRequest{FromSession: from})
		if got := workspaceOf(f, d.ID); got != workspaces.DefaultName {
			t.Errorf("from %q: workspace = %q, want Default", from, got)
		}
	}
}

func TestChatDraftCreate_FolderSourceFromADesignIsTheTrunkCheckout(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	design := designSession(t, f, "Maps")
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderSource, FromSession: design})
	if d.Dir != f.Repo {
		t.Errorf("dir = %q, want trunk checkout %q", d.Dir, f.Repo)
	}
}

func TestChatDraftCreate_FolderSourceFromAChatIsTheChatsDir(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	chat, dir := plainChat(t, f, "")
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderSource, FromSession: chat})
	if d.Dir == "" || d.Dir != dir {
		t.Errorf("dir = %q, want %q", d.Dir, dir)
	}
}

func TestChatDraftSave_FolderBackToSourceKeepsTheSourceChatsFolder(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	chat, dir := plainChat(t, f, "")
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderSource, FromSession: chat})
	rev := d.Revision
	save := func(folder string) *ops.ChatDraft {
		t.Helper()
		res := post(t, f, "save", ops.ChatDraftSaveRequest{ID: d.ID, Brief: "b", Revision: rev, Folder: str(folder)})
		if res.Refused || res.Draft == nil {
			t.Fatalf("save %s = %+v", folder, res)
		}
		rev = res.Draft.Revision
		return res.Draft
	}
	if got := save(ops.ChatDraftFolderTrunk); got.Dir != f.Repo {
		t.Fatalf("trunk dir = %q, want %q", got.Dir, f.Repo)
	}
	if got := save(ops.ChatDraftFolderSource); got.Dir != dir {
		t.Errorf("source dir after the round trip = %q, want the chat's %q", got.Dir, dir)
	}
}

func TestChatDraftCreate_FolderTrunk(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderTrunk})
	if d.Dir != f.Repo || d.Worktree != "" {
		t.Errorf("dir = %q worktree = %q, want %q and none", d.Dir, d.Worktree, f.Repo)
	}
}

func TestChatDraftCreate_FolderWorktreeStoresANameAndNoPath(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	named := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderWorktree, Worktree: "fix-it"})
	if named.Worktree != "fix-it" || named.Dir != "" || named.WorktreePath != "" {
		t.Errorf("named = %+v", named)
	}
	auto := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderWorktree, Name: "Fix The Bug!"})
	if want := "fix-the-bug-" + auto.ID[:6]; auto.Worktree != want {
		t.Errorf("auto worktree = %q, want %q", auto.Worktree, want)
	}
	if res := create(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderWorktree, Worktree: "../out"}); !res.Refused || res.Reason != wire.ReasonSessionBadRequest {
		t.Errorf("non-local worktree = %+v", res)
	}
}

func TestChatDraftCreate_RefusalsStoreNothing(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	cases := map[string]struct {
		req    ops.ChatDraftCreateRequest
		reason string
	}{
		"over 32 KiB":     {ops.ChatDraftCreateRequest{Brief: strings.Repeat("x", wire.MaxSessionPromptBytes+1)}, wire.ReasonPromptTextTooLong},
		"bang prefix":     {ops.ChatDraftCreateRequest{Brief: "!ls"}, wire.ReasonPromptPrefixNotAllowed},
		"unknown project": {ops.ChatDraftCreateRequest{Project: "nope"}, wire.ReasonSessionBadRequest},
		"unknown harness": {ops.ChatDraftCreateRequest{Harness: "nope"}, wire.ReasonSessionUnknownHarness},
		"effort no model": {ops.ChatDraftCreateRequest{Effort: "high"}, wire.ReasonSessionBadRequest},
		"unknown source":  {ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderSource, FromSession: "nope"}, wire.ReasonSessionBadRequest},
	}
	for name, c := range cases {
		res := create(t, f, c.req)
		if !res.Refused || res.Reason != c.reason || res.Draft != nil {
			t.Errorf("%s: result = %+v, want refused %s", name, res, c.reason)
		}
	}
	if list, err := chatdrafts.New(f.H.StateDir).List(); err != nil || len(list) != 0 {
		t.Errorf("stored drafts = %+v, %v", list, err)
	}
}

func TestChatDraftRead_GoneIDIsDraftGone(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	if res := post(t, f, "read", ops.ChatDraftReadRequest{ID: "0123456789abcdef"}); !res.Refused || res.Reason != ops.ReasonDraftGone {
		t.Errorf("read = %+v", res)
	}
}

func TestChatDraftSave_StaleRevisionIsAConflictAndOtherFieldsStay(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Name: "keep", Brief: "first"})
	ok := post(t, f, "save", ops.ChatDraftSaveRequest{ID: d.ID, Brief: "second", Revision: 1})
	if ok.Draft == nil || ok.Draft.Revision != 2 || ok.Draft.Brief != "second" || ok.Draft.Name != "keep" || ok.Draft.Dir != d.Dir {
		t.Fatalf("save = %+v", ok)
	}
	stale := post(t, f, "save", ops.ChatDraftSaveRequest{ID: d.ID, Brief: "third", Revision: 1})
	if !stale.Refused || stale.Reason != ops.ReasonDraftRevisionConflict {
		t.Errorf("stale save = %+v", stale)
	}
	if gone := post(t, f, "save", ops.ChatDraftSaveRequest{ID: "0123456789abcdef", Revision: 1}); gone.Reason != ops.ReasonDraftGone {
		t.Errorf("gone save = %+v", gone)
	}
}

func TestChatDraftDelete_RemovesTheRecordAndTheMember(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	chat, _ := plainChat(t, f, "Bugs")
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{FromSession: chat})
	if res := post(t, f, "delete", ops.ChatDraftReadRequest{ID: d.ID}); !res.Deleted {
		t.Fatalf("delete = %+v", res)
	}
	if res := post(t, f, "read", ops.ChatDraftReadRequest{ID: d.ID}); res.Reason != ops.ReasonDraftGone {
		t.Errorf("read after delete = %+v", res)
	}
	if got := workspaceOf(f, d.ID); got != workspaces.DefaultName {
		t.Errorf("member after delete in %q", got)
	}
	if res := post(t, f, "delete", ops.ChatDraftReadRequest{ID: d.ID}); !res.Refused || res.Reason != ops.ReasonDraftGone {
		t.Errorf("second delete = %+v", res)
	}
}

func str(s string) *string { return &s }

func TestChatDraftCreate_EmptyBriefInANamedWorkspace(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	if _, err := f.H.Client.Admin.CreateWorkspace(f.Ctx, "Ideas"); err != nil {
		t.Fatal(err)
	}
	body := ops.ChatDraftCreateRequest{Project: "proj", Folder: ops.ChatDraftFolderSource, Workspace: "Ideas"}
	res := post(t, f, "create", body)
	if res.Refused || res.Draft == nil || res.Draft.Brief != "" || res.Draft.Label != "Untitled chat" {
		t.Fatalf("create = %+v", res)
	}
	if res.Draft.Dir != f.Repo {
		t.Errorf("source with no session: dir = %q, want project root %q", res.Draft.Dir, f.Repo)
	}
	if got := workspaceOf(f, res.Draft.ID); got != "Ideas" {
		t.Errorf("workspace = %q, want Ideas", got)
	}
	body.Workspace = ""
	if d := post(t, f, "create", body).Draft; d == nil || workspaceOf(f, d.ID) != workspaces.DefaultName {
		t.Errorf("no workspace: %+v", d)
	}
}

func TestChatDraftCreate_FromSessionWinsOverWorkspace(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	chat, _ := plainChat(t, f, "Bugs")
	if _, err := f.H.Client.Admin.CreateWorkspace(f.Ctx, "Ideas"); err != nil {
		t.Fatal(err)
	}
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{FromSession: chat, Workspace: "Ideas"})
	if got := workspaceOf(f, d.ID); got != "Bugs" {
		t.Errorf("workspace = %q, want Bugs", got)
	}
}

func TestChatDraftStart_EmptyBriefIsRefused(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	d := post(t, f, "create", ops.ChatDraftCreateRequest{Project: "proj", Folder: ops.ChatDraftFolderTrunk}).Draft
	if res := post(t, f, "start", ops.ChatDraftReadRequest{ID: d.ID}); !res.Refused || res.Reason != wire.ReasonEmptyPrompt {
		t.Errorf("start = %+v", res)
	}
}

func TestChatDraftSave_EachSettingAloneStoresAndBumpsTheRevision(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Name: "keep", Model: "opus", Effort: "high"})
	rev := d.Revision
	save := func(req ops.ChatDraftSaveRequest) *ops.ChatDraft {
		t.Helper()
		req.ID, req.Brief, req.Revision = d.ID, "b", rev
		res := post(t, f, "save", req)
		if res.Refused || res.Draft == nil || res.Draft.Revision != rev+1 {
			t.Fatalf("save %+v = %+v", req, res)
		}
		rev++
		return res.Draft
	}
	if got := save(ops.ChatDraftSaveRequest{Name: str("renamed")}); got.Name != "renamed" || got.Model != "opus" || got.Folder != d.Folder {
		t.Errorf("name only: %+v", got)
	}
	if got := save(ops.ChatDraftSaveRequest{Harness: str("claude"), Model: str("sonnet"), Effort: str("low")}); got.Harness != "claude" || got.Model != "sonnet" || got.Effort != "low" || got.Name != "renamed" {
		t.Errorf("harness/model/effort: %+v", got)
	}
	if got := save(ops.ChatDraftSaveRequest{Folder: str(ops.ChatDraftFolderSource)}); got.Folder != ops.ChatDraftFolderSource || got.Dir != f.Repo {
		t.Errorf("folder only: %+v", got)
	}
	if got := save(ops.ChatDraftSaveRequest{Project: str("proj")}); got.Project != "proj" || got.Dir != f.Repo || got.Model != "sonnet" {
		t.Errorf("project only: %+v", got)
	}
	if got := save(ops.ChatDraftSaveRequest{Folder: str(ops.ChatDraftFolderWorktree), Worktree: str("wt-1")}); got.Worktree != "wt-1" || got.Dir != "" {
		t.Errorf("worktree: %+v", got)
	}
}

func TestChatDraftSave_FolderDirTakesAFolderOfTheProject(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{})
	res := post(t, f, "save", ops.ChatDraftSaveRequest{
		ID: d.ID, Brief: "b", Revision: d.Revision,
		Folder: str(ops.ChatDraftFolderDir), Dir: str(f.Repo),
	})
	if res.Refused || res.Draft.Folder != ops.ChatDraftFolderDir || res.Draft.Dir != f.Repo {
		t.Fatalf("save dir = %+v", res)
	}
}

func TestChatDraftSave_RefusalsStoreNothing(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Name: "keep"})
	cases := map[string]struct {
		req    ops.ChatDraftSaveRequest
		reason string
	}{
		"bad harness": {ops.ChatDraftSaveRequest{Harness: str("nope")}, wire.ReasonSessionUnknownHarness},
		"bad project": {ops.ChatDraftSaveRequest{Project: str("nope")}, wire.ReasonSessionBadRequest},
		"bad folder":  {ops.ChatDraftSaveRequest{Folder: str("moon")}, wire.ReasonSessionBadRequest},
		"dir outside": {ops.ChatDraftSaveRequest{Folder: str(ops.ChatDraftFolderDir), Dir: str("/etc")}, wire.ReasonSessionBadRequest},
		"stale":       {ops.ChatDraftSaveRequest{Name: str("x"), Revision: 9}, ops.ReasonDraftRevisionConflict},
	}
	for name, c := range cases {
		c.req.ID, c.req.Brief = d.ID, "changed"
		if c.req.Revision == 0 {
			c.req.Revision = 1
		}
		if res := post(t, f, "save", c.req); !res.Refused || res.Reason != c.reason {
			t.Errorf("%s: %+v, want refused %s", name, res, c.reason)
		}
	}
	got := post(t, f, "read", ops.ChatDraftReadRequest{ID: d.ID}).Draft
	if got.Revision != 1 || got.Brief != d.Brief || got.Name != "keep" || got.Harness != d.Harness {
		t.Errorf("draft changed: %+v", got)
	}
}

func TestChatDraftEvents_CreateAndDeletePublishOneEachBriefSaveNone(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	var d *ops.ChatDraft
	if n := draftEvents(t, f, func() { d = mustCreate(t, f, ops.ChatDraftCreateRequest{}) }); n != 1 {
		t.Errorf("create published %d events, want 1", n)
	}
	if n := draftEvents(t, f, func() { post(t, f, "save", ops.ChatDraftSaveRequest{ID: d.ID, Brief: "b", Revision: 1}) }); n != 0 {
		t.Errorf("brief save published %d events, want 0", n)
	}
	if n := draftEvents(t, f, func() {
		post(t, f, "save", ops.ChatDraftSaveRequest{ID: d.ID, Brief: "b", Revision: 2, Name: str("new")})
	}); n != 1 {
		t.Errorf("settings save published %d events, want 1", n)
	}
	if n := draftEvents(t, f, func() { post(t, f, "delete", ops.ChatDraftReadRequest{ID: d.ID}) }); n != 1 {
		t.Errorf("delete published %d events, want 1", n)
	}
}

func TestChatDraftImages_UploadedImageIsServedOnTheWebMux(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{})
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 16)
	code, out := servertest.WebPostJSON(t, f.Base+"/v1/agents/drafts/images?id="+d.ID, png, f.Ck, "", servertest.JSONPost(f.Base))
	var up wire.SessionImageResult
	if code != http.StatusOK || json.Unmarshal([]byte(out), &up) != nil || up.ID == "" {
		t.Fatalf("upload = %d %s", code, out)
	}
	code, body := servertest.WebDoCookie(t, http.MethodGet, f.Base+"/v1/agents/drafts/images?id="+d.ID+"&image="+up.ID, f.Ck)
	if code != http.StatusOK || body != png {
		t.Fatalf("serve = %d, %d bytes, want the %d uploaded", code, len(body), len(png))
	}
}
