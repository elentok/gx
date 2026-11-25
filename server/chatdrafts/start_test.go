package chatdrafts

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

type fakeWorkspaces struct {
	mu      sync.Mutex
	members map[string]string
}

func (f *fakeWorkspaces) WorkspaceOfSession(string) string { return "Default" }
func (f *fakeWorkspaces) SetMember(key, ws string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.members[key] = ws
	return nil
}
func (f *fakeWorkspaces) Remove(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.members, key)
	return nil
}
func (f *fakeWorkspaces) Workspace(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ws, ok := f.members[key]; ok {
		return ws
	}
	return "Default"
}
func (f *fakeWorkspaces) has(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.members[key]
	return ok
}

type startRig struct {
	c         *ChatDrafts
	ws        *fakeWorkspaces
	events    *broker.Broker
	reqs      []sessions.WebCreateSessionRequest
	reqMu     sync.Mutex
	createRes func() (wire.SessionResult, error)
	chats     atomic.Int32
	worktrees atomic.Int32
	wtRefusal string
	stateDir  string
}

func newStartRig(t *testing.T) *startRig {
	t.Helper()
	r := &startRig{ws: &fakeWorkspaces{members: map[string]string{}}, events: broker.New(16)}
	r.createRes = func() (wire.SessionResult, error) {
		return wire.SessionResult{Session: &wire.SessionInfo{ID: "sess-1"}}, nil
	}
	r.stateDir = t.TempDir()
	r.c = NewChatDrafts(Deps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), StateDir: r.stateDir,
		History: drafthistory.New(r.stateDir), Events: r.events, Workspaces: r.ws,
		CreateChat: func(req sessions.WebCreateSessionRequest) (wire.SessionResult, error) {
			r.chats.Add(1)
			r.reqMu.Lock()
			r.reqs = append(r.reqs, req)
			r.reqMu.Unlock()
			return r.createRes()
		},
		AddWorktree: func(project, name string) (string, string) {
			r.worktrees.Add(1)
			if r.wtRefusal != "" {
				return "", r.wtRefusal
			}
			return "/wt/" + name, ""
		},
	})
	return r
}

func (r *startRig) draft(t *testing.T, d ops.ChatDraft) *ops.ChatDraft {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	d.ID = id
	if d.Project == "" {
		d.Project = "proj"
	}
	if d.Brief == "" {
		d.Brief = "do it"
	}
	if d.Folder == "" {
		d.Folder, d.Dir = ops.ChatDraftFolderTrunk, "/trunk"
	}
	if err := r.c.store.Create(&d); err != nil {
		t.Fatal(err)
	}
	if err := r.ws.SetMember(MemberKey(id), "Bugs"); err != nil {
		t.Fatal(err)
	}
	return &d
}

func TestStart_CreateRequestCarriesTheDraftsSettings(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	d := r.draft(t, ops.ChatDraft{Name: "n", Harness: "claude", Model: "m", Effort: "high", Brief: "the brief"})
	if res, err := r.c.Start(d.ID); err != nil || res.Refused {
		t.Fatalf("start = %+v, %v", res, err)
	}
	want := sessions.WebCreateSessionRequest{Project: "proj", Dir: "/trunk", Name: "n", Prompt: "the brief",
		Harness: "claude", Model: "m", Effort: "high", Workspace: "Bugs"}
	if len(r.reqs) != 1 || !reflect.DeepEqual(r.reqs[0], want) {
		t.Fatalf("requests = %+v, want %+v", r.reqs, want)
	}
}

func TestStart_SuccessDeletesTheDraftDropsTheMemberRecordsTheStartAndPublishesOnce(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	d := r.draft(t, ops.ChatDraft{})
	before := r.events.CurrentSeq()
	res, err := r.c.Start(d.ID)
	if err != nil || res.Started != "sess-1" || res.Refused {
		t.Fatalf("start = %+v, %v", res, err)
	}
	if r.c.store.Exists(d.ID) || r.ws.has(MemberKey(d.ID)) {
		t.Error("draft or member survived a successful Start")
	}
	if got := r.c.store.Started(d.ID); got != "sess-1" {
		t.Errorf("start record = %q", got)
	}
	if n := r.events.CurrentSeq() - before; n != 1 {
		t.Errorf("events = %d, want 1", n)
	}
}

func TestStart_FailureKeepsTheDraftRecordsTheErrorAndPublishesNothing(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	r.createRes = func() (wire.SessionResult, error) {
		return wire.SessionResult{Refused: true, Reason: wire.ReasonSessionBadRequest, Message: "no good"}, nil
	}
	d := r.draft(t, ops.ChatDraft{})
	before := r.events.CurrentSeq()
	res, err := r.c.Start(d.ID)
	if err != nil || !res.Refused || res.Reason != wire.ReasonSessionBadRequest || res.Message != "no good" {
		t.Fatalf("start = %+v, %v", res, err)
	}
	got, err := r.c.store.Get(d.ID)
	if err != nil || got.StartError != "no good" || !r.ws.has(MemberKey(d.ID)) {
		t.Fatalf("draft = %+v, %v", got, err)
	}
	if r.events.CurrentSeq() != before {
		t.Error("a failed Start published an event")
	}
	r.createRes = func() (wire.SessionResult, error) { return wire.SessionResult{}, errors.New("boom") }
	if res, _ := r.c.Start(d.ID); res.Reason != ops.ReasonChatDraftStartFailed || res.Message != "boom" {
		t.Errorf("error start = %+v", res)
	}
}

func TestStart_WorktreeIsMadeOnceAndTheRetryReusesIt(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	r.createRes = func() (wire.SessionResult, error) { return wire.SessionResult{}, errors.New("boom") }
	d := r.draft(t, ops.ChatDraft{Folder: ops.ChatDraftFolderWorktree, Worktree: "wt"})
	if res, _ := r.c.Start(d.ID); !res.Refused {
		t.Fatalf("first start = %+v", res)
	}
	if got, _ := r.c.store.Get(d.ID); got.WorktreePath != "/wt/wt" {
		t.Fatalf("worktree_path = %q", got.WorktreePath)
	}
	r.createRes = func() (wire.SessionResult, error) {
		return wire.SessionResult{Session: &wire.SessionInfo{ID: "sess-2"}}, nil
	}
	if res, _ := r.c.Start(d.ID); res.Started != "sess-2" {
		t.Fatalf("retry = %+v", res)
	}
	if n := r.worktrees.Load(); n != 1 {
		t.Errorf("AddWorktree calls = %d, want 1", n)
	}
	if last := r.reqs[len(r.reqs)-1]; last.Dir != "/wt/wt" {
		t.Errorf("retry dir = %q", last.Dir)
	}
}

func TestStart_WorktreeFailureKeepsTheDraftAndMakesNoChat(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	r.wtRefusal = "branch exists"
	d := r.draft(t, ops.ChatDraft{Folder: ops.ChatDraftFolderWorktree, Worktree: "wt"})
	res, _ := r.c.Start(d.ID)
	if !res.Refused || res.Message != "branch exists" {
		t.Fatalf("start = %+v", res)
	}
	got, err := r.c.store.Get(d.ID)
	if err != nil || got.StartError != "branch exists" || got.WorktreePath != "" || r.chats.Load() != 0 {
		t.Fatalf("draft = %+v, %v, chats = %d", got, err, r.chats.Load())
	}
}

func TestStart_GoneDraftCarriesStartedOnlyAfterAStart(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	id, _ := NewID()
	if res, _ := r.c.Start(id); res.Reason != ops.ReasonDraftGone || res.Started != "" {
		t.Fatalf("never started = %+v", res)
	}
	d := r.draft(t, ops.ChatDraft{})
	if _, err := r.c.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	if res, _ := r.c.Start(d.ID); res.Reason != ops.ReasonDraftGone || res.Started != "sess-1" {
		t.Fatalf("after start = %+v", res)
	}
}

func TestStart_EmptyBriefIsRefusedAndTheDraftStays(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	d := r.draft(t, ops.ChatDraft{})
	if _, err := r.c.store.SaveChecked(ops.ChatDraftSaveRequest{ID: d.ID, Brief: "", Revision: 1}, nil); err != nil {
		t.Fatal(err)
	}
	res, _ := r.c.Start(d.ID)
	if !res.Refused || res.Reason != wire.ReasonEmptyPrompt || !r.c.store.Exists(d.ID) || r.chats.Load() != 0 {
		t.Fatalf("start = %+v", res)
	}
}

func TestStart_RemoveFailureAfterTheChatStillReturnsStartedAndASecondStartOpensNoChat(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	d := r.draft(t, ops.ChatDraft{})
	draftDir := r.c.store.dir(d.ID)
	r.createRes = func() (wire.SessionResult, error) {
		// A read-only draft dir makes RemoveAll fail once the chat exists.
		if err := os.Chmod(draftDir, 0o500); err != nil {
			t.Fatal(err)
		}
		return wire.SessionResult{Session: &wire.SessionInfo{ID: "sess-1"}}, nil
	}
	t.Cleanup(func() { _ = os.Chmod(draftDir, 0o700) })
	res, err := r.c.Start(d.ID)
	if err != nil || res.Started != "sess-1" || res.Refused {
		t.Fatalf("first start = %+v, %v", res, err)
	}
	if !r.c.store.Exists(d.ID) {
		t.Fatal("test setup: the remove did not fail")
	}
	if err := os.Chmod(draftDir, 0o700); err != nil {
		t.Fatal(err)
	}
	res, err = r.c.Start(d.ID)
	if err != nil || res.Started != "sess-1" || r.chats.Load() != 1 {
		t.Fatalf("second start = %+v, %v, chats = %d", res, err, r.chats.Load())
	}
	if r.c.store.Exists(d.ID) {
		t.Error("the second start did not clean the draft up")
	}
}

func TestStart_TwoConcurrentStartsCreateOneChat(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	d := r.draft(t, ops.ChatDraft{})
	var wg sync.WaitGroup
	results := make([]ops.ChatDraftResult, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = r.c.Start(d.ID)
		}()
	}
	wg.Wait()
	if n := r.chats.Load(); n != 1 {
		t.Fatalf("chats = %d, want 1", n)
	}
	for _, res := range results {
		if res.Started != "sess-1" {
			t.Errorf("result = %+v, want started sess-1", res)
		}
	}
}

func TestStart_NilEditWritesNothingAndABriefEditBumpsTheRevisionOnce(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	r.createRes = func() (wire.SessionResult, error) {
		return wire.SessionResult{Refused: true, Reason: wire.ReasonSessionBadRequest, Message: "no good"}, nil
	}
	d := r.draft(t, ops.ChatDraft{})
	if _, err := r.c.StartWithEdit(d.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.c.store.Get(d.ID); got.Revision != d.Revision || got.Brief != "do it" {
		t.Fatalf("after nil edit = %+v, want revision %d", got, d.Revision)
	}
	if _, err := r.c.StartWithEdit(d.ID, &ops.ChatDraftEdit{Brief: "edited"}); err != nil {
		t.Fatal(err)
	}
	got, _ := r.c.store.Get(d.ID)
	if got.Revision != d.Revision+1 || got.Brief != "edited" || got.StartError != "no good" {
		t.Fatalf("after edit = %+v", got)
	}
	if n := len(r.reqs); n != 2 || r.reqs[1].Prompt != "edited" {
		t.Errorf("requests = %+v", r.reqs)
	}
}

func TestStart_EditToAnEmptyBriefIsRefusedAndTheDraftStays(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	d := r.draft(t, ops.ChatDraft{})
	res, _ := r.c.StartWithEdit(d.ID, &ops.ChatDraftEdit{Brief: ""})
	if !res.Refused || res.Reason != wire.ReasonEmptyPrompt || r.chats.Load() != 0 {
		t.Fatalf("start = %+v", res)
	}
	if got, err := r.c.store.Get(d.ID); err != nil || got.Brief != "" {
		t.Errorf("draft = %+v, %v", got, err)
	}
}

func TestStart_EditIsIgnoredForAGoneOrAlreadyStartedDraft(t *testing.T) {
	t.Parallel()
	r := newStartRig(t)
	edit := &ops.ChatDraftEdit{Brief: "edited"}
	if res, _ := r.c.StartWithEdit("nosuchdraft", edit); res.Reason != ops.ReasonDraftGone || res.Started != "" {
		t.Fatalf("gone = %+v", res)
	}
	d := r.draft(t, ops.ChatDraft{})
	if _, err := r.c.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	if res, _ := r.c.StartWithEdit(d.ID, edit); res.Reason != ops.ReasonDraftGone || res.Started != "sess-1" {
		t.Fatalf("started = %+v", res)
	}
	if r.chats.Load() != 1 {
		t.Errorf("chats = %d, want 1", r.chats.Load())
	}
}
