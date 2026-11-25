package chatdrafts_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

func sameDir(a, b string) bool {
	ra, _ := filepath.EvalSymlinks(a)
	rb, _ := filepath.EvalSymlinks(b)
	return ra == rb
}

// rootChat makes a chat in the project root, a dir the web create check accepts.
func rootChat(t *testing.T, f rigs.DesignsFixture) wire.SessionInfo {
	t.Helper()
	res, err := f.H.Client.Sessions.Create(f.Ctx, wire.CreateSessionRequest{Project: "proj", Dir: f.Repo})
	if err != nil || res.Session == nil {
		t.Fatalf("create chat = %+v, %v", res, err)
	}
	return *res.Session
}

func startOK(t *testing.T, f rigs.DesignsFixture, id string) (string, map[string]any) {
	t.Helper()
	res := post(t, f, "start", ops.ChatDraftReadRequest{ID: id})
	if res.Refused || res.Started == "" {
		t.Fatalf("start = %+v", res)
	}
	return res.Started, servertest.ReadMeta(t, f.H, res.Started)
}

func TestChatDraftStart_SourceFolderRunsInTheSourcesDir(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	src := rootChat(t, f)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderSource, FromSession: src.ID,
		Model: "opus", Effort: "high", Name: "child"})
	sid, meta := startOK(t, f, d.ID)
	if dir, _ := meta["dir"].(string); !sameDir(dir, src.Dir) {
		t.Errorf("dir = %v, want %s", meta["dir"], src.Dir)
	}
	if meta["model"] != "opus" || meta["effort"] != "high" || meta["first_prompt"] != "do the thing" || meta["name"] != "child" {
		t.Errorf("meta = %+v", meta)
	}
	if got := post(t, f, "read", ops.ChatDraftReadRequest{ID: d.ID}); got.Reason != ops.ReasonDraftGone || got.Started != sid {
		t.Errorf("old read = %+v, want gone and started %s", got, sid)
	}
}

func TestChatDraftStart_SourceFromADesignRunsInTheTrunkNotTheDesign(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	ds := designSession(t, f, "Bugs")
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderSource, FromSession: ds})
	_, meta := startOK(t, f, d.ID)
	dir, _ := meta["dir"].(string)
	if !sameDir(dir, f.Repo) {
		t.Errorf("dir = %s, want trunk %s", dir, f.Repo)
	}
}

func TestChatDraftStart_TrunkFolderLandsInTheDraftsWorkspaceAndDropsTheMember(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	src, _ := plainChat(t, f, "Bugs")
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{FromSession: src})
	sid, meta := startOK(t, f, d.ID)
	if dir, _ := meta["dir"].(string); !sameDir(dir, f.Repo) {
		t.Errorf("dir = %s, want %s", dir, f.Repo)
	}
	if got := f.H.Server.Workspaces().Workspace(sid); got != "Bugs" {
		t.Errorf("chat workspace = %q, want Bugs", got)
	}
	if got := workspaceOf(f, d.ID); got == "Bugs" {
		t.Errorf("draft member still in %q", got)
	}
}

func TestChatDraftStart_WorktreeFolderRunsInANewWorktree(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderWorktree, Worktree: "feat-x"})
	_, meta := startOK(t, f, d.ID)
	dir, _ := meta["dir"].(string)
	if filepath.Base(dir) != "feat-x" || sameDir(dir, f.Repo) {
		t.Errorf("dir = %s, want a new worktree named feat-x", dir)
	}
	all, err := f.H.Client.Reads.ProjectWorktrees(f.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := slices.ContainsFunc(all, func(p wire.ProjectWorktrees) bool {
		return p.Project == "proj" && slices.ContainsFunc(p.Worktrees, func(w string) bool { return sameDir(w, dir) })
	})
	if !found {
		t.Errorf("worktrees = %+v, missing %s", all, dir)
	}
}

func TestChatDraftStart_FailureKeepsTheDraftAndShowsTheError(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	src, _ := plainChat(t, f, "") // its dir is neither the root nor a worktree
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderSource, FromSession: src})
	res := post(t, f, "start", ops.ChatDraftReadRequest{ID: d.ID})
	if !res.Refused || res.Reason != wire.ReasonSessionBadRequest {
		t.Fatalf("start = %+v", res)
	}
	got := post(t, f, "read", ops.ChatDraftReadRequest{ID: d.ID})
	if got.Draft == nil || got.Draft.StartError == "" || got.Draft.StartError != res.Message {
		t.Errorf("read = %+v", got)
	}
}

func TestChatDraftStart_EditAppliesTheBriefAndSettingsAndPublishesOnce(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{})
	var res ops.ChatDraftResult
	events := draftEvents(t, f, func() {
		res = post(t, f, "start", ops.ChatDraftStartRequest{ID: d.ID,
			Edit: &ops.ChatDraftEdit{Brief: "edited brief", Name: str(" renamed "), Model: str("opus"), Effort: str("high")}})
	})
	if res.Refused || res.Started == "" {
		t.Fatalf("start = %+v", res)
	}
	meta := servertest.ReadMeta(t, f.H, res.Started)
	if meta["first_prompt"] != "edited brief" || meta["name"] != "renamed" || meta["model"] != "opus" || meta["effort"] != "high" {
		t.Errorf("meta = %+v", meta)
	}
	if events != 1 {
		t.Errorf("events = %d, want 1", events)
	}
}

func TestChatDraftStart_RefusedEditAnswersLikeASaveAndKeepsTheDraft(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	d := mustCreate(t, f, ops.ChatDraftCreateRequest{})
	var res ops.ChatDraftResult
	events := draftEvents(t, f, func() {
		res = post(t, f, "start", ops.ChatDraftStartRequest{ID: d.ID,
			Edit: &ops.ChatDraftEdit{Brief: "edited", Effort: str("high")}})
	})
	save := post(t, f, "save", ops.ChatDraftSaveRequest{ID: d.ID, Brief: "edited", Revision: d.Revision, Effort: str("high")})
	if !res.Refused || res.Reason != save.Reason || res.Message != save.Message || res.Reason == "" {
		t.Fatalf("start = %+v, save = %+v", res, save)
	}
	got := post(t, f, "read", ops.ChatDraftReadRequest{ID: d.ID}).Draft
	if got == nil || got.Brief != d.Brief || got.Revision != d.Revision || events != 0 {
		t.Errorf("draft = %+v, events = %d", got, events)
	}
}

func TestChatDraftStart_FailedStartKeepsTheEditAndPublishesOnlyForASettingsEdit(t *testing.T) {
	t.Parallel()
	f := rigs.StartDesigns(t)
	src, _ := plainChat(t, f, "") // its dir is neither the root nor a worktree
	for name, c := range map[string]struct {
		edit   ops.ChatDraftEdit
		events int
	}{
		"settings": {ops.ChatDraftEdit{Brief: "settings edit", Name: str("kept")}, 1},
		"brief":    {ops.ChatDraftEdit{Brief: "brief edit"}, 0},
	} {
		d := mustCreate(t, f, ops.ChatDraftCreateRequest{Folder: ops.ChatDraftFolderSource, FromSession: src})
		var res ops.ChatDraftResult
		events := draftEvents(t, f, func() {
			res = post(t, f, "start", ops.ChatDraftStartRequest{ID: d.ID, Edit: &c.edit})
		})
		got := post(t, f, "read", ops.ChatDraftReadRequest{ID: d.ID}).Draft
		if !res.Refused || got == nil || got.Brief != c.edit.Brief || got.StartError == "" || got.StartError != res.Message {
			t.Errorf("%s: start = %+v, draft = %+v", name, res, got)
		}
		if events != c.events {
			t.Errorf("%s: events = %d, want %d", name, events, c.events)
		}
	}
}
