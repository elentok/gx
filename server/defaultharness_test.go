package server_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func setHarness(t *testing.T, h *servertest.Harness, req server.SetDefaultHarnessRequest) server.SetDefaultHarnessResult {
	t.Helper()
	res, err := h.Client.SetDefaultHarness(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func agentsKey(m map[string]any) any {
	agents, _ := m["agents"].(map[string]any)
	return agents["default-harness"]
}

func TestDefaultHarness_GlobalWriteKeepsKeysAndSymlink(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	target := filepath.Join(t.TempDir(), "real.json")
	link := filepath.Join(t.TempDir(), "config.json")
	servertest.WriteConfig(t, target, store, map[string]any{"agents": map[string]any{"claude": map[string]any{"model": "opus"}}})
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(link))

	res := setHarness(t, h, server.SetDefaultHarnessRequest{Scope: "global", Harness: "codex"})
	if res.Refused || !res.Changed || res.Harness != "codex" {
		t.Fatalf("result = %+v", res)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config path is no longer a symlink: %v, %v", fi, err)
	}
	got := readJSONFile(t, target)
	if agentsKey(got) != "codex" {
		t.Errorf("target = %v", got)
	}
	if _, ok := got["ticket-store"]; !ok {
		t.Errorf("other keys lost: %v", got)
	}
	if claude := got["agents"].(map[string]any)["claude"]; claude == nil {
		t.Errorf("sibling agents key lost: %v", got)
	}
	st, err := h.Client.DefaultHarness(context.Background())
	if err != nil || st.Default != "codex" {
		t.Errorf("status after reload = %+v, %v", st, err)
	}
}

func TestDefaultHarness_GlobalCreatesMissingFileAndRefusesEmpty(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))

	if res := setHarness(t, h, server.SetDefaultHarnessRequest{Scope: "global"}); !res.Refused || res.Reason != server.ReasonNoGlobalHarnessSet {
		t.Errorf("empty global = %+v, want refusal", res)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a refused write created the file")
	}
	if res := setHarness(t, h, server.SetDefaultHarnessRequest{Scope: "global", Harness: "codex"}); res.Path != path {
		t.Fatalf("result = %+v", res)
	}
	if got := agentsKey(readJSONFile(t, path)); got != "codex" {
		t.Errorf("file = %v", got)
	}
}

func TestDefaultHarness_ProjectWriteAndRemoveOverride(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "one", servertest.TicketOpts{})
	pj := filepath.Join(store, "proj", "project.json")
	if err := os.WriteFile(pj, []byte(`{"vcs":"git","checks":{"test":false}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)

	res := setHarness(t, h, server.SetDefaultHarnessRequest{Scope: "project", Project: "proj", Harness: "codex"})
	if res.Refused || !res.Changed || res.Harness != "codex" {
		t.Fatalf("set = %+v", res)
	}
	got := readJSONFile(t, pj)
	if agentsKey(got) != "codex" || got["vcs"] != "git" || got["checks"] == nil || len(got["agents"].(map[string]any)) != 1 {
		t.Errorf("project.json = %v", got)
	}
	st, _ := h.Client.DefaultHarness(context.Background())
	if !slices.Contains(st.Projects, server.ProjectDefaultHarness{Project: "proj", Harness: "codex"}) {
		t.Errorf("status = %+v", st)
	}

	res = setHarness(t, h, server.SetDefaultHarnessRequest{Scope: "project", Project: "proj"})
	if res.Refused || !res.Changed || res.Harness != "claude" {
		t.Fatalf("remove = %+v", res)
	}
	got = readJSONFile(t, pj)
	if _, ok := got["agents"]; ok || got["vcs"] != "git" || got["checks"] == nil {
		t.Errorf("project.json after remove = %v", got)
	}
	if res := setHarness(t, h, server.SetDefaultHarnessRequest{Scope: "project", Project: "nope", Harness: "codex"}); !res.Refused || res.Reason != server.ReasonUnknownProject {
		t.Errorf("unknown project = %+v", res)
	}
}

func TestDefaultHarness_RefusesHarnessNotOnPath(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "one", servertest.TicketOpts{})
	path := filepath.Join(t.TempDir(), "config.json")
	servertest.WriteConfig(t, path, store, nil)
	pj := filepath.Join(store, "proj", "project.json")
	if err := os.WriteFile(pj, []byte(`{"vcs":"git"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	h.Programs.Missing("codex")
	cfgBefore, _ := os.ReadFile(path)
	pjBefore, _ := os.ReadFile(pj)

	for _, req := range []server.SetDefaultHarnessRequest{
		{Scope: "global", Harness: "codex"},
		{Scope: "project", Project: "proj", Harness: "codex"},
	} {
		res := setHarness(t, h, req)
		if !res.Refused || res.Reason != server.ReasonHarnessNotOnPath || !strings.Contains(res.Message, "codex is not on PATH") {
			t.Errorf("%+v = %+v", req, res)
		}
	}
	if after, _ := os.ReadFile(path); string(after) != string(cfgBefore) {
		t.Errorf("config changed: %s", after)
	}
	if after, _ := os.ReadFile(pj); string(after) != string(pjBefore) {
		t.Errorf("project.json changed: %s", after)
	}
}

func TestDefaultHarness_NoticeCounts(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "inherit", "epic", "01", "waits", servertest.TicketOpts{})
	servertest.WriteTicketWith(t, store, "inherit", "epic", "02", "pinned", servertest.TicketOpts{Agent: "claude"})
	servertest.WriteTicketWith(t, store, "override", "epic", "01", "waits", servertest.TicketOpts{})
	servertest.WriteTicketWith(t, store, "idle", "epic", "01", "not queued", servertest.TicketOpts{})
	if err := os.WriteFile(filepath.Join(store, "override", "project.json"), []byte(`{"agents":{"default-harness":"claude"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	servertest.WriteConfig(t, path, store, nil)
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	ctx := context.Background()
	if _, err := h.Client.QueuePause(ctx); err != nil {
		t.Fatal(err)
	}
	for _, addr := range []string{"inherit:epic/01", "override:epic/01"} {
		if res, err := h.Client.QueueAdd(ctx, addr, ""); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}

	res := setHarness(t, h, server.SetDefaultHarnessRequest{Scope: "global", Harness: "codex"})
	if want := "Default is now Codex. 1 waiting ticket and the next design step will use it."; res.Notice != want || res.Waiting != 1 {
		t.Errorf("global = %+v, want notice %q", res, want)
	}
	if res := setHarness(t, h, server.SetDefaultHarnessRequest{Scope: "global", Harness: "codex"}); res.Changed || res.Notice != "" {
		t.Errorf("no change = %+v", res)
	}
	res = setHarness(t, h, server.SetDefaultHarnessRequest{Scope: "project", Project: "override", Harness: "codex"})
	if want := "Default for override is now Codex. 1 waiting ticket and the next design step will use it."; res.Notice != want {
		t.Errorf("project = %+v, want notice %q", res, want)
	}
	res = setHarness(t, h, server.SetDefaultHarnessRequest{Scope: "project", Project: "idle", Harness: "claude"})
	if want := "Default for idle is now Claude. The next design step will use it."; res.Notice != want || res.Waiting != 0 {
		t.Errorf("zero waiting = %+v, want notice %q", res, want)
	}
}
