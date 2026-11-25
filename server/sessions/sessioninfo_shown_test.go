package sessions_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

// commonDirCounter wraps the real lookup and counts calls per directory.
type commonDirCounter struct {
	mu    sync.Mutex
	calls map[string]int
}

func (c *commonDirCounter) option() func(*server.Config) {
	return func(cfg *server.Config) {
		cfg.DiffCommonDir = func(dir string) (string, error) {
			c.mu.Lock()
			c.calls[dir]++
			c.mu.Unlock()
			_, common, err := git.GitDirs(dir)
			return common, err
		}
	}
}

func (c *commonDirCounter) count(dir string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[dir]
}

func TestSessionInfo_ShowDiffDirFollowsTheProjectRepo(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	repo, linked := testutil.TempRepoWithLinkedWorktree(t, "feature")
	clone := testutil.TempRepo(t)
	plain := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic", "01", "t", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	counter := &commonDirCounter{calls: map[string]int{}}
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), counter.option())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cases := []struct {
		name, dir, fixOf string
		want             bool
	}{
		{"repo", repo, "", true},
		{"linked worktree", linked, "", true},
		{"separate clone", clone, "", false},
		{"non-repo dir", plain, "", false},
		{"fix agent in the repo", repo, "proj:epic/01", true},
		{"fix agent in a clone", clone, "proj:epic/02", false},
	}
	ids := map[string]string{}
	for _, c := range cases {
		res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{
			Project: "proj", Dir: c.dir, Name: c.name, FixOf: c.fixOf, Harness: string(agentfake.HarnessKind),
		})
		if err != nil || res.Session == nil {
			t.Fatalf("create %s: %+v, %v", c.name, res, err)
		}
		ids[c.name] = res.Session.ID
	}

	// Several list reads, then one git check per directory.
	var snap wire.Snapshot
	for range 3 {
		var err error
		if snap, err = h.Client.Snapshot(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range cases {
		var got *wire.SessionInfo
		for i := range snap.Sessions {
			if snap.Sessions[i].ID == ids[c.name] {
				got = &snap.Sessions[i]
			}
		}
		if got == nil {
			t.Fatalf("%s: session %s missing from the snapshot", c.name, ids[c.name])
		}
		if got.ShowDiffDir != c.want {
			t.Errorf("%s: ShowDiffDir = %v, want %v", c.name, got.ShowDiffDir, c.want)
		}
	}
	for _, dir := range []string{repo, linked, clone, plain} {
		if n := counter.count(dir); n != 1 {
			t.Errorf("common-dir lookups for %s = %d, want 1", dir, n)
		}
	}
}

func TestSessionInfoModel_ShownModelFollowsTheSwitchTime(t *testing.T) {
	t.Parallel()
	switchedAt := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	older, newer := switchedAt.Add(-time.Minute).Format(time.RFC3339), switchedAt.Add(time.Minute).Format(time.RFC3339)
	claudeReply := func(at string) string {
		return `{"type":"assistant","timestamp":"` + at + `","message":{"model":"claude-opus-5-5","usage":{"input_tokens":10}}}`
	}
	codexTurn := func(at string) string {
		return `{"type":"turn_context","timestamp":"` + at + `","payload":{"model":"gpt-6.1-sol","effort":"low"}}`
	}
	writeRollout := func(t *testing.T, line string) string {
		t.Helper()
		home := t.TempDir()
		dir := filepath.Join(home, "sessions", "2026", "10", "30")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "rollout-conv.jsonl"), []byte(line+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return home
	}
	claudeMeta := sessions.Meta{Harness: "claude", Dir: "/work/proj", ConversationID: "conv", Tier: "worker", Model: "claude-sonnet-5-5", Effort: "high", SwitchedAt: switchedAt}
	codexMeta := sessions.Meta{Harness: "codex", ConversationID: "conv", Tier: "worker", Model: "gpt-6.1-terra", Effort: "high", SwitchedAt: switchedAt}

	for name, tc := range map[string]struct {
		info                  func(t *testing.T) wire.SessionInfo
		wantModel, wantEffort string
	}{
		"claude before a newer turn shows the record": {func(t *testing.T) wire.SessionInfo {
			return modelTestServer(t, "/work/proj", claudeReply(older)).SessionInfo(claudeMeta)
		}, "claude-sonnet-5-5", "high"},
		"claude after a newer turn shows the reply and the record's effort": {func(t *testing.T) wire.SessionInfo {
			return modelTestServer(t, "/work/proj", claudeReply(newer)).SessionInfo(claudeMeta)
		}, "claude-opus-5-5", "high"},
		"codex before a newer turn shows the record": {func(t *testing.T) wire.SessionInfo {
			return infoManager(nil, writeRollout(t, codexTurn(older))).SessionInfo(codexMeta)
		}, "gpt-6.1-terra", "high"},
		"codex after a newer turn shows the reported model and effort": {func(t *testing.T) wire.SessionInfo {
			return infoManager(nil, writeRollout(t, codexTurn(newer))).SessionInfo(codexMeta)
		}, "gpt-6.1-sol", "low"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			info := tc.info(t)
			if info.Model != tc.wantModel || info.Effort != tc.wantEffort {
				t.Errorf("model/effort = %q/%q, want %s/%s", info.Model, info.Effort, tc.wantModel, tc.wantEffort)
			}
		})
	}

	// A switch back to the tier's model shows no flag before the next turn.
	back := claudeMeta
	back.Model = "claude-opus-5-5"
	if modelTestServer(t, "/work/proj", claudeReply(older)).SessionInfo(back).ModelSwitched {
		t.Error("switch back to the tier model: ModelSwitched = true, want false")
	}
}
