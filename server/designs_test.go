package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

type designsFixture struct {
	h    *servertest.Harness
	ctx  context.Context
	evs  <-chan server.Event
	repo string
	base string
	ck   *http.Cookie
}

// startDesigns starts a server with a map epic "map" and a plain epic "plain"
// in project proj, whose repo is the trunk checkout, with the web mux up.
func startDesigns(t *testing.T, opts ...func(*server.Config)) designsFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return startDesignsIn(t, store, opts...)
}

// startDesignsIn is startDesigns on a store the caller has already seeded.
func startDesignsIn(t *testing.T, store string, opts ...func(*server.Config)) designsFixture {
	t.Helper()
	repo := testutil.TempRepo(t)
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	opts = append([]func(*server.Config){servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)),
		func(c *server.Config) { c.WebAddr = "127.0.0.1:0" }}, opts...)
	h := servertest.StartWithStore(t, store, opts...)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	hs, err := h.Client.Handshake(ctx)
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	return designsFixture{h: h, ctx: ctx, evs: evs, repo: repo, base: hs.WebURL, ck: login(t, h.StateDir, hs.WebURL)}
}

func (f designsFixture) open(t *testing.T, epic string, isNew bool) server.SessionResult {
	t.Helper()
	res, err := f.h.Client.OpenDesign(f.ctx, server.DesignOpenRequest{Epic: epic, New: isNew})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (f designsFixture) live(t *testing.T) []server.SessionInfo {
	t.Helper()
	snap, err := f.h.Client.Snapshot(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var out []server.SessionInfo
	for _, s := range snap.Sessions {
		if s.Status.State != server.SessionExited {
			out = append(out, s)
		}
	}
	return out
}

func TestDesignsOpen_CreatesWithTheTuiPrefill(t *testing.T) {
	f := startDesigns(t)
	res := f.open(t, "proj:map", false)
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	meta := readMeta(t, f.h, res.Session.ID)
	dir, _ := meta["dir"].(string)
	want := map[string]any{"dir": dir, "name": "map", "ticket": "proj:map", "harness": string(agentfake.HarnessKind), "first_prompt": "/gx-design proj:map", "design": true, "made_worktree": true}
	for k, v := range want {
		if meta[k] != v {
			t.Errorf("meta[%s] = %v, want %v", k, meta[k], v)
		}
	}
	if dir == f.repo || gitOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD") != "map-design" {
		t.Errorf("dir %s is not a map-design worktree", dir)
	}
	if gitOut(t, dir, "rev-parse", "HEAD") != gitOut(t, f.repo, "rev-parse", "HEAD") {
		t.Errorf("map-design does not start at trunk")
	}
}

// startDesignsAt is startDesigns with the map's design_step set (none when empty).
func startDesignsAt(t *testing.T, step string) designsFixture {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "map-impl", "01", "first", "")
	fm := "---\nkind: map\n"
	if step != "" {
		fm += "design_step: " + step + "\n"
	}
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte(fm+"---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return startDesignsIn(t, store)
}

func TestDesignsOpen_AfterBranchStartsInTheTrunkCheckout(t *testing.T) {
	for _, tc := range []struct{ name, step, implBranch string }{
		{"step branch", "branch", ""},
		{"step done", "done", ""},
		{"impl branch only", "create-tickets", "map-impl"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := startDesignsAt(t, tc.step)
			if tc.implBranch != "" {
				gitOut(t, f.repo, "branch", tc.implBranch)
			}
			res := f.open(t, "proj:map", false)
			if res.Refused || res.Session == nil {
				t.Fatalf("open = %+v", res)
			}
			meta := readMeta(t, f.h, res.Session.ID)
			if dir, _ := meta["dir"].(string); dir != f.repo {
				t.Errorf("dir = %q, want trunk checkout %q", dir, f.repo)
			}
			if meta["made_worktree"] == true {
				t.Errorf("made_worktree = true, want false")
			}
			if out := gitOut(t, f.repo, "branch", "--list", "map-design"); out != "" {
				t.Errorf("map-design branch made: %q", out)
			}
		})
	}
}

func TestDesignsOpen_ImplEpicWithoutBranchStillUsesTheDesignWorktree(t *testing.T) {
	f := startDesignsAt(t, "create-tickets")
	res := f.open(t, "proj:map", false)
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	dir, _ := readMeta(t, f.h, res.Session.ID)["dir"].(string)
	if gitOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD") != "map-design" {
		t.Errorf("dir %s is not the map-design worktree", dir)
	}
}

// reopenAfterTrunkCommit opens the map, stops it, lets edit change the design
// worktree, commits file=content on trunk, and opens again.
func (f designsFixture) reopenAfterTrunkCommit(t *testing.T, file, content string, edit func(dir string)) (server.SessionResult, string) {
	t.Helper()
	first := f.open(t, "proj:map", false)
	dir, _ := readMeta(t, f.h, first.Session.ID)["dir"].(string)
	if _, err := f.h.Client.StopSession(f.ctx, first.Session.ID); err != nil {
		t.Fatal(err)
	}
	edit(dir)
	if err := os.WriteFile(filepath.Join(f.repo, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, f.repo, "add", file)
	gitOut(t, f.repo, "commit", "-m", "trunk change")
	res := f.open(t, "proj:map", false)
	if res.Refused || res.Session == nil {
		t.Fatalf("reopen = %+v", res)
	}
	return res, dir
}

func TestDesignsOpen_RebasesTheDesignBranchOntoTrunk(t *testing.T) {
	f := startDesigns(t)
	res, dir := f.reopenAfterTrunkCommit(t, "t.txt", "trunk\n", func(string) {})
	trunk := gitOut(t, f.repo, "rev-parse", "HEAD")
	if gitOut(t, dir, "merge-base", "--is-ancestor", trunk, "HEAD") != "" {
		t.Errorf("trunk is not an ancestor of map-design")
	}
	if res.Session.RebaseNote != "" {
		t.Errorf("note = %q, want none", res.Session.RebaseNote)
	}
}

func TestDesignsOpen_ConflictSkipsTheRebaseAndSetsANote(t *testing.T) {
	f := startDesigns(t)
	var tip string
	res, dir := f.reopenAfterTrunkCommit(t, "c.txt", "trunk\n", func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("design\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitOut(t, dir, "add", "c.txt")
		gitOut(t, dir, "commit", "-m", "design change")
		tip = gitOut(t, dir, "rev-parse", "HEAD")
	})
	if got := gitOut(t, dir, "rev-parse", "HEAD"); got != tip {
		t.Errorf("tip = %s, want unchanged %s", got, tip)
	}
	if out := gitOut(t, dir, "status", "--porcelain"); out != "" {
		t.Errorf("worktree not clean after abort: %s", out)
	}
	if !strings.Contains(res.Session.RebaseNote, "not rebased") || !strings.Contains(res.Session.RebaseNote, "conflict") {
		t.Errorf("note = %q", res.Session.RebaseNote)
	}
	if note, _ := readMeta(t, f.h, res.Session.ID)["rebase_note"].(string); note != res.Session.RebaseNote {
		t.Errorf("meta note = %q", note)
	}
}

func TestLinkSession_CarriesTheRebaseNote(t *testing.T) {
	f := startDesigns(t)
	res, _ := f.reopenAfterTrunkCommit(t, "c.txt", "trunk\n", func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("design\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitOut(t, dir, "add", "c.txt")
		gitOut(t, dir, "commit", "-m", "design change")
	})
	linked, err := f.h.Client.LinkSession(f.ctx, server.LinkSessionRequest{ID: res.Session.ID, Ticket: "proj:map"})
	if err != nil || linked.Session == nil || linked.Session.RebaseNote != res.Session.RebaseNote || linked.Session.RebaseNote == "" {
		t.Errorf("link = %+v, %v; want the open's note %q", linked.Session, err, res.Session.RebaseNote)
	}
}

func TestDesignsOpen_UncommittedEditsSkipTheRebaseAndSetANote(t *testing.T) {
	f := startDesigns(t)
	var before string
	res, dir := f.reopenAfterTrunkCommit(t, "t.txt", "trunk\n", func(dir string) {
		before = gitOut(t, dir, "rev-parse", "HEAD")
		if err := os.WriteFile(filepath.Join(dir, "wip.txt"), []byte("wip\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	if got := gitOut(t, dir, "rev-parse", "HEAD"); got != before {
		t.Errorf("tip moved to %s from %s", got, before)
	}
	if !strings.Contains(res.Session.RebaseNote, "uncommitted") {
		t.Errorf("note = %q", res.Session.RebaseNote)
	}
}

func TestDesignLiveCheck_LeavesOutTheCallerAndRebaseSkipsForAnother(t *testing.T) {
	f := startDesigns(t)
	first := f.open(t, "proj:map", false)
	id := first.Session.ID
	dir, _ := readMeta(t, f.h, id)["dir"].(string)
	s := f.h.Server
	if got := s.LiveDesignSessionID("proj:map", ""); got != id {
		t.Fatalf("live = %q, want %q", got, id)
	}
	if got := s.LiveDesignSessionID("proj:map", id); got != "" {
		t.Errorf("live leaving out the caller = %q, want none", got)
	}
	if note := s.RebaseDesign("proj", "map", dir, ""); !strings.Contains(note, "live") {
		t.Errorf("note = %q, want another-live-session note", note)
	}
	if note := s.RebaseDesign("proj", "map", dir, id); note != "" {
		t.Errorf("note = %q, want a clean rebase", note)
	}
}

func TestDesignsOpen_ReAddsARemovedWorktreeOnTheSameBranch(t *testing.T) {
	f := startDesigns(t)
	first := f.open(t, "proj:map", false)
	dir, _ := readMeta(t, f.h, first.Session.ID)["dir"].(string)
	if _, err := f.h.Client.StopSession(f.ctx, first.Session.ID); err != nil {
		t.Fatal(err)
	}
	gitOut(t, dir, "commit", "--allow-empty", "-m", "design work")
	tip := gitOut(t, dir, "rev-parse", "HEAD")
	gitOut(t, f.repo, "worktree", "remove", "--force", dir)

	second := f.open(t, "proj:map", false)
	if second.Refused || second.Session == nil {
		t.Fatalf("open = %+v", second)
	}
	got, _ := readMeta(t, f.h, second.Session.ID)["dir"].(string)
	if got != dir || gitOut(t, got, "rev-parse", "--abbrev-ref", "HEAD") != "map-design" || gitOut(t, got, "rev-parse", "HEAD") != tip {
		t.Errorf("worktree %s not back on map-design at %s", got, tip)
	}
}

func TestDesignsOpen_ClashRefusalKeepsTheLiveSession(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := startDesignsIn(t, store)
	first := f.open(t, "proj:map", false)
	if first.Refused || first.Session == nil {
		t.Fatalf("open = %+v", first)
	}
	servertest.WriteTicket(t, store, "proj", "map-design", "01", "first", "")

	res := f.open(t, "proj:map", true)
	if !res.Refused || res.Reason != server.ReasonDesignBranchClash {
		t.Fatalf("open = %+v, want design-branch-clash", res)
	}
	live := f.live(t)
	if len(live) != 1 || live[0].ID != first.Session.ID {
		t.Errorf("live sessions = %+v, want only %s", live, first.Session.ID)
	}
}

func TestDesignsOpen_RefusesAnEpicNamedLikeTheDesignBranch(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "map-design", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := startDesignsIn(t, store)
	res := f.open(t, "proj:map", false)
	if !res.Refused || res.Reason != server.ReasonDesignBranchClash || !strings.Contains(res.Message, "map-design") {
		t.Fatalf("open = %+v, want design-branch-clash naming map-design", res)
	}
	if gitOut(t, f.repo, "branch", "--list", "map-design") != "" {
		t.Errorf("branch map-design was created")
	}
}

func (f designsFixture) worktree(t *testing.T, epic, exclude string) server.DesignWorktreeResult {
	t.Helper()
	res, err := f.h.Client.DesignWorktree(f.ctx, server.DesignWorktreeRequest{Epic: epic, ExcludeSession: exclude})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// commitOnTrunk commits file=content on the trunk checkout.
func (f designsFixture) commitOnTrunk(t *testing.T, file, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.repo, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, f.repo, "add", file)
	gitOut(t, f.repo, "commit", "-m", "trunk change")
}

func TestDesignsWorktree_ReturnsTheOpenPathAndRebasesOnAMovedTrunk(t *testing.T) {
	f := startDesigns(t)
	first := f.worktree(t, "proj:map", "")
	if first.Refused || first.Note != "" || first.AfterBranch {
		t.Fatalf("worktree = %+v", first)
	}
	if gitOut(t, first.Path, "rev-parse", "--abbrev-ref", "HEAD") != "map-design" {
		t.Errorf("path %s is not the map-design worktree", first.Path)
	}
	f.commitOnTrunk(t, "t.txt", "trunk\n")
	second := f.worktree(t, "proj:map", "")
	if second.Path != first.Path || second.Note != "" {
		t.Errorf("worktree = %+v, want path %s and no note", second, first.Path)
	}
	if gitOut(t, second.Path, "merge-base", "--is-ancestor", gitOut(t, f.repo, "rev-parse", "HEAD"), "HEAD") != "" {
		t.Errorf("trunk is not an ancestor of map-design")
	}
	opened := f.open(t, "proj:map", false)
	if dir, _ := readMeta(t, f.h, opened.Session.ID)["dir"].(string); dir != first.Path {
		t.Errorf("open dir = %q, want %q", dir, first.Path)
	}
}

func TestDesignsWorktree_ConflictReturnsTheNote(t *testing.T) {
	f := startDesigns(t)
	dir := f.worktree(t, "proj:map", "").Path
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("design\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, dir, "add", "c.txt")
	gitOut(t, dir, "commit", "-m", "design change")
	f.commitOnTrunk(t, "c.txt", "trunk\n")
	res := f.worktree(t, "proj:map", "")
	if res.Path != dir || !strings.Contains(res.Note, "not rebased") || !strings.Contains(res.Note, "conflict") {
		t.Errorf("worktree = %+v", res)
	}
}

func TestDesignsWorktree_AfterBranchReturnsTheTrunkCheckoutAndMakesNothing(t *testing.T) {
	f := startDesignsAt(t, "branch")
	res := f.worktree(t, "proj:map", "")
	if res.Refused || res.Path != f.repo || !res.AfterBranch || res.Note != "" {
		t.Errorf("worktree = %+v, want trunk checkout %s after Branch", res, f.repo)
	}
	if out := gitOut(t, f.repo, "branch", "--list", "map-design"); out != "" {
		t.Errorf("map-design branch made: %q", out)
	}
}

func TestDesignsWorktree_Refusals(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "map-design", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := startDesignsIn(t, store)
	for _, tc := range []struct{ epic, reason string }{
		{"proj:plain", server.ReasonDesignNotMapEpic},
		{"proj:nope", server.ReasonDesignUnknownEpic},
		{"proj:map", server.ReasonDesignBranchClash},
		{"proj", server.ReasonSessionBadRequest},
	} {
		if res := f.worktree(t, tc.epic, ""); !res.Refused || res.Reason != tc.reason || res.Path != "" {
			t.Errorf("worktree %s = %+v, want %s", tc.epic, res, tc.reason)
		}
	}
	if gitOut(t, f.repo, "branch", "--list", "map-design") != "" {
		t.Errorf("branch map-design was created")
	}
}

func TestDesignsWorktree_LeavesOutTheCallersSessionFromTheLiveCheck(t *testing.T) {
	f := startDesigns(t)
	id := f.open(t, "proj:map", false).Session.ID
	f.commitOnTrunk(t, "t.txt", "trunk\n")
	if res := f.worktree(t, "proj:map", ""); !strings.Contains(res.Note, "live") {
		t.Errorf("note = %q, want another-live-session note", res.Note)
	}
	if res := f.worktree(t, "proj:map", id); res.Note != "" {
		t.Errorf("note = %q, want a clean rebase", res.Note)
	}
}

func TestDesignsOpen_ReturnsTheNewestLiveSession(t *testing.T) {
	f := startDesigns(t)
	mk := func() string {
		res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: "proj:map"})
		if err != nil || res.Session == nil {
			t.Fatalf("create = %+v, %v", res, err)
		}
		return res.Session.ID
	}
	mk()
	newest := mk()
	if res := f.open(t, "proj:map", false); res.Session == nil || res.Session.ID != newest {
		t.Errorf("open = %+v, want session %s", res, newest)
	}
}

func TestDesignsOpen_ARestoredSessionCountsFromItsRestore(t *testing.T) {
	f := startDesigns(t)
	mk := func() string {
		res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: "proj:map"})
		if err != nil || res.Session == nil {
			t.Fatalf("create = %+v, %v", res, err)
		}
		return res.Session.ID
	}
	older := mk()
	mk()
	metaPath := filepath.Join(f.h.SessionsDir, older, "meta.json")
	meta := readMeta(t, f.h, older)
	meta["restored_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if res := f.open(t, "proj:map", false); res.Session == nil || res.Session.ID != older {
		t.Errorf("open = %+v, want the restored session %s", res, older)
	}
}

func TestDesignsOpen_ConcurrentCallsMakeOneSession(t *testing.T) {
	f := startDesigns(t)
	ids := make([]string, 4)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if res, err := f.h.Client.OpenDesign(f.ctx, server.DesignOpenRequest{Epic: "proj:map"}); err == nil && res.Session != nil {
				ids[i] = res.Session.ID
			}
		}()
	}
	wg.Wait()
	for _, id := range ids {
		if id == "" || id != ids[0] {
			t.Fatalf("ids = %v, want one shared session", ids)
		}
	}
	if n := len(f.live(t)); n != 1 {
		t.Errorf("%d live sessions, want 1", n)
	}
}

func TestDesignsOpen_NewStopsTheOldAndCreatesAnother(t *testing.T) {
	f := startDesigns(t)
	old := f.open(t, "proj:map", false).Session
	waitForHolder(t, filepath.Join(f.h.SessionsDir, old.ID))
	res := f.open(t, "proj:map", true)
	if res.Refused || res.Session == nil || res.Session.ID == old.ID {
		t.Fatalf("new = %+v, old %s", res, old.ID)
	}
	snap, err := f.h.Client.Snapshot(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Sessions {
		if s.ID == old.ID && s.Status.State != server.SessionExited {
			t.Errorf("old session is %s, want Exited", s.Status.State)
		}
	}
}

func TestDesignsOpen_TicketAddressPromptsOnCreateAndSendsNothingToALiveSession(t *testing.T) {
	f := startDesigns(t)
	res := f.open(t, "proj:map/01", false)
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	if p := readMeta(t, f.h, res.Session.ID)["first_prompt"]; p != "/gx-design proj:map/01" {
		t.Errorf("first prompt = %v", p)
	}
	again := f.open(t, "proj:map/02", false)
	if again.Session == nil || again.Session.ID != res.Session.ID {
		t.Errorf("open on a live Design = %+v, want session %s", again, res.Session.ID)
	}
	if p := readMeta(t, f.h, res.Session.ID)["first_prompt"]; p != "/gx-design proj:map/01" {
		t.Errorf("first prompt changed to %v", p)
	}
}

func TestDesignsOpen_Refusals(t *testing.T) {
	f := startDesigns(t)
	for epic, want := range map[string]string{
		"proj:plain":   server.ReasonDesignNotMapEpic,
		"proj:nope":    server.ReasonDesignUnknownEpic,
		"nope:map":     server.ReasonDesignUnknownEpic,
		"map":          server.ReasonSessionBadRequest,
		"proj:map/abc": server.ReasonSessionBadRequest,
	} {
		if res := f.open(t, epic, false); !res.Refused || res.Reason != want {
			t.Errorf("open %s = %+v, want %s", epic, res, want)
		}
	}
}

func TestWebDesigns_NewNeedsAFreshToken(t *testing.T) {
	f := startDesigns(t)
	live := f.open(t, "proj:map", false).Session
	waitForHolder(t, filepath.Join(f.h.SessionsDir, live.ID))
	post := func(body string) (int, string) {
		return webPostJSON(t, f.base+"/v1/designs/open", body, f.ck, "", jsonPost(f.base))
	}

	for name, body := range map[string]string{
		"no token":  `{"epic":"proj:map","new":true}`,
		"old token": fmt.Sprintf(`{"epic":"proj:map","new":true,"state":%q}`, live.ID+":starting:1"),
	} {
		if code, out := post(body); code != http.StatusOK || reasonOf(t, out) != server.ReasonStale {
			t.Errorf("new with %s = %d %s, want stale", name, code, out)
		}
	}
	if code, out := post(`{"epic":"proj:map"}`); code != http.StatusOK || reasonOf(t, out) != "" {
		t.Errorf("plain call = %d %s, want it to need no token", code, out)
	}
	fresh := fmt.Sprintf(`{"epic":"proj:map","new":true,"state":%q}`, sessionToken(f.ctx, t, f.h, live.ID))
	if code, out := post(fresh); code != http.StatusOK || reasonOf(t, out) != "" {
		t.Errorf("new with a fresh token = %d %s, want it to succeed", code, out)
	}
}
