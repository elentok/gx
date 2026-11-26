package infoflows_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/commandlist"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	sessprompt "github.com/elentok/gx/server/sessions/prompt"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/transcript"
)

// latestManager is a Manager whose model cache holds the claude and codex
// lists, read by fake probes. A nil list leaves that harness with no list.
func latestManager(t *testing.T, claude, codex []commandlist.Model, cwd string, lines ...string) *sessions.Manager {
	t.Helper()
	return latestManagerIn(t, "", claude, codex, cwd, lines...)
}

// latestManagerIn is latestManager over the ticket store at store, so a
// session's project can carry a project.json.
func latestManagerIn(t *testing.T, store string, claude, codex []commandlist.Model, cwd string, lines ...string) *sessions.Manager {
	t.Helper()
	cfgDir := t.TempDir()
	if len(lines) > 0 {
		path := filepath.Join(cfgDir, "projects", transcript.Slugify(cwd), "conv.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	probe := func(list []commandlist.Model) commandlist.ModelProbe {
		return func(context.Context, string, []string) ([]commandlist.Model, error) {
			if list == nil {
				return nil, context.Canceled
			}
			return list, nil
		}
	}
	cache := commandlist.NewModelCache(commandlist.ModelCacheDeps{
		Claude: probe(claude), Codex: probe(codex),
		NewTicker: func(time.Duration) (<-chan time.Time, func()) { return nil, func() {} },
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { cache.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, c := cache.Models("claude")
		_, x := cache.Models("codex")
		if (c || claude == nil) && (x || codex == nil) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the model cache")
		}
		<-time.After(time.Millisecond)
	}

	set := &settings.Holder{}
	set.Store(&settings.Settings{
		AgentsClaude: config.AgentConfig{
			Model: "fable", Effort: "high",
			Light:     config.TierConfig{Model: "claude-haiku-5-1"},
			Strongest: config.TierConfig{Model: "claude-fable-5-1", Effort: "max"},
		},
		AgentsCodex: config.AgentConfig{Model: "gpt-6.1-terra"},
	})
	log := slog.New(slog.DiscardHandler)
	return sessions.NewManager(sessions.ManagerDeps{
		Chat:     chat.New(chat.Deps{Config: chat.Config{Env: []string{"CLAUDE_CONFIG_DIR=" + cfgDir}}, Source: infoSource{t.TempDir()}, Log: log}),
		Models:   cache,
		Prompter: sessprompt.New(sessprompt.Deps{Log: log}),
		Settings: set,
		Projects: projects.NewRegistry(func() projects.Env { return projects.Env{TicketStore: store} }, log, nil, set, nil, nil),
	})
}

func TestSessionInfo_LatestTiersFollowProjectOverride(t *testing.T) {
	t.Parallel()
	claudeList := []commandlist.Model{
		{Value: "claude-fable-5-1", ID: "claude-fable-5-1", Efforts: []string{"high"}},
		{Value: "claude-fable-5-2", ID: "claude-fable-5-2", Efforts: []string{"high"}},
		{Value: "claude-haiku-5-1", ID: "claude-haiku-5-1", Efforts: []string{"low"}},
	}
	store := t.TempDir()
	dir := filepath.Join(store, "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	override := `{"name":"proj","agents":{"claude":{"model":"claude-haiku-5-1","strongest":{"model":"claude-haiku-5-1"}}}}`
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(override), 0o644); err != nil {
		t.Fatal(err)
	}
	const cwd = "/work/proj"
	mg := latestManagerIn(t, store, claudeList, nil, cwd)
	meta := sessions.Meta{Harness: "claude", Dir: cwd, ConversationID: "conv", Model: "claude-fable-5-1", Project: "proj"}

	if got := mg.SessionInfo(meta).LatestTiers; len(got) != 0 {
		t.Errorf("LatestTiers with a project override = %v, want none (the override moved the tiers off this model)", got)
	}
	meta.Project = ""
	if got := mg.SessionInfo(meta).LatestTiers; !slices.Equal(got, []string{"strongest"}) {
		t.Errorf("LatestTiers without a project = %v, want [strongest] from the global settings", got)
	}
}

func TestSessionInfo_LatestModel(t *testing.T) {
	t.Parallel()
	claudeList := []commandlist.Model{
		{Value: "fable", ID: "claude-fable-5-1", Efforts: []string{"low", "medium", "high"}},
		{Value: "claude-fable-5-1", ID: "claude-fable-5-1", Efforts: []string{"low", "medium", "high"}},
		{Value: "claude-fable-5-2", ID: "claude-fable-5-2", Efforts: []string{"low", "medium", "high", "xhigh"}},
		{Value: "claude-haiku-5-1", ID: "claude-haiku-5-1", Efforts: []string{"low"}},
	}
	codexList := []commandlist.Model{
		{Value: "gpt-6.1-terra", ID: "gpt-6.1-terra", Efforts: []string{"low", "medium"}, DefaultEffort: "medium"},
		{Value: "gpt-6.2-terra", ID: "gpt-6.2-terra", Efforts: []string{"high"}, DefaultEffort: "high"},
	}
	const cwd = "/work/proj"
	reply := func(model string) string { return replyWithUsage(model, 100) }
	claude := func(model, effort string) sessions.Meta {
		return sessions.Meta{Harness: "claude", Dir: cwd, ConversationID: "conv", Model: model, Effort: effort}
	}

	for name, tc := range map[string]struct {
		mg             *sessions.Manager
		meta           sessions.Meta
		wantLatest     string
		wantFamily     string
		wantTiers      []string
		wantEffort     string
		wantHasHistory bool
	}{
		"a Claude id with a newer id": {
			latestManager(t, claudeList, nil, cwd, reply("claude-fable-5-1")), claude("", "xhigh"),
			"claude-fable-5-2", "fable", []string{"worker", "strongest"}, "xhigh", true,
		},
		"a lagging alias gets the latest before any reply": {
			latestManager(t, claudeList, nil, cwd), claude("fable", "max"),
			"claude-fable-5-2", "fable", []string{"worker", "strongest"}, "xhigh", false,
		},
		"the latest model gets none": {
			latestManager(t, claudeList, nil, cwd, reply("claude-fable-5-2")), claude("", "high"),
			"", "", nil, "", true,
		},
		"an unparseable model gets none": {
			latestManager(t, claudeList, nil, cwd, reply("mystery")), claude("", "high"),
			"", "", nil, "", true,
		},
		"no list gets none": {
			latestManager(t, nil, nil, cwd, reply("claude-fable-5-1")), claude("", "high"),
			"", "", nil, "", true,
		},
		"codex takes the new model's default when the effort is not supported": {
			latestManager(t, nil, codexList, cwd), sessions.Meta{Harness: "codex", Model: "gpt-6.1-terra", Effort: "low"},
			"gpt-6.2-terra", "terra", []string{"worker"}, "high", false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			info := tc.mg.SessionInfo(tc.meta)
			if info.LatestModel != tc.wantLatest || info.LatestFamily != tc.wantFamily || info.LatestEffort != tc.wantEffort || !slices.Equal(info.LatestTiers, tc.wantTiers) {
				t.Errorf("latest/family/tiers/effort = %q/%q/%v/%q, want %q/%q/%v/%q", info.LatestModel, info.LatestFamily, info.LatestTiers, info.LatestEffort, tc.wantLatest, tc.wantFamily, tc.wantTiers, tc.wantEffort)
			}
			if info.HasHistory != tc.wantHasHistory {
				t.Errorf("HasHistory = %v, want %v", info.HasHistory, tc.wantHasHistory)
			}
		})
	}
}
