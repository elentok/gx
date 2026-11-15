package modelflows_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/commandlist"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

// The servertest harness runs this test binary as its session holder and as
// the fake herdr; without both calls the binary re-runs the suite in every
// child and forks without end.
func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	herdrfake.RunHelperProcess()
	os.Exit(m.Run())
}

var modelsClaude = []commandlist.Model{
	{Value: "default"},
	{Value: "opus", ID: "claude-opus-4-1", Efforts: []string{"low", "medium", "high"}},
	{Value: "claude-opus-4-1", ID: "claude-opus-4-1", Efforts: []string{"low", "medium", "high"}},
	{Value: "claude-opus-4-5", ID: "claude-opus-4-5", Efforts: []string{"low", "medium"}},
	{Value: "claude-opus-3-9", ID: "claude-opus-3-9", Hidden: true},
	{Value: "custom", ID: "custom"},
}

// modelsServer starts a server whose Claude model probe answers claude (an
// error when nil) and counts its calls. block, when set, holds every probe
// until it is closed.
func modelsServer(t *testing.T, claude []commandlist.Model, block chan struct{}) (rigs.DesignsFixture, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	probe := func(ctx context.Context, _ string, _ []string) ([]commandlist.Model, error) {
		calls.Add(1)
		if block != nil {
			select {
			case <-block:
			case <-ctx.Done():
			}
		}
		if claude == nil {
			return nil, context.Canceled
		}
		return claude, nil
	}
	f := rigs.StartDesigns(t, func(c *server.Config) {
		c.Models = commandlist.NewModelCache(commandlist.ModelCacheDeps{Claude: probe, Codex: func(context.Context, string, []string) ([]commandlist.Model, error) {
			return []commandlist.Model{}, nil
		}})
	})
	return f, &calls
}

func seedModelSession(t *testing.T, f rigs.DesignsFixture, id, harnessName, model, effort string) {
	t.Helper()
	dir := filepath.Join(f.H.SessionsDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(map[string]any{
		"id": id, "harness": harnessName, "project": "proj", "dir": t.TempDir(),
		"model": model, "effort": effort, "switched_at": time.Now(),
	})
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
}

func models(t *testing.T, f rigs.DesignsFixture, id string) (int, wire.SessionModelsResponse, string) {
	t.Helper()
	code, body := servertest.WebDoCookie(t, http.MethodGet, f.Base+"/v1/sessions/models?session="+url.QueryEscape(id), f.Ck)
	var res wire.SessionModelsResponse
	if code == http.StatusOK {
		if err := json.Unmarshal([]byte(body), &res); err != nil {
			t.Fatalf("body %q: %v", body, err)
		}
	}
	return code, res, body
}

// modelsOnceListed polls until the list has rows.
func modelsOnceListed(t *testing.T, f rigs.DesignsFixture, id string, want int) wire.SessionModelsResponse {
	t.Helper()
	var res wire.SessionModelsResponse
	if !testutil.Poll(10*time.Second, 10*time.Millisecond, func() bool {
		_, res, _ = models(t, f, id)
		return len(res.Models) >= want
	}) {
		t.Fatalf("models = %+v, want %d rows", res, want)
	}
	return res
}

func TestWebSessions_ModelsByHarnessNeedsNoSession(t *testing.T) {
	t.Parallel()
	f, _ := modelsServer(t, modelsClaude, nil)
	var res wire.SessionModelsResponse
	if !testutil.Poll(10*time.Second, 10*time.Millisecond, func() bool {
		code, body := servertest.WebDoCookie(t, http.MethodGet, f.Base+"/v1/sessions/models?harness=claude", f.Ck)
		if code != http.StatusOK {
			t.Fatalf("code = %d, body %q", code, body)
		}
		if err := json.Unmarshal([]byte(body), &res); err != nil {
			t.Fatalf("body %q: %v", body, err)
		}
		return len(res.Aliases) > 0
	}) {
		t.Fatalf("aliases = %+v", res.Aliases)
	}
	if res.Aliases[0] != (wire.ModelAlias{Alias: "opus", ID: "claude-opus-4-1", Latest: "claude-opus-4-5"}) {
		t.Errorf("aliases = %+v", res.Aliases)
	}
}

func TestWebSessions_ModelsListsVisibleResolvedRows(t *testing.T) {
	t.Parallel()
	f, _ := modelsServer(t, modelsClaude, nil)
	seedModelSession(t, f, "s1", "claude", "opus", "high")

	res := modelsOnceListed(t, f, "s1", 3)
	var ids []string
	byID := map[string]wire.SessionModelRow{}
	for _, m := range res.Models {
		ids = append(ids, m.ID)
		byID[m.ID] = m
	}
	if want := []string{"claude-opus-4-1", "claude-opus-4-5", "custom"}; fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Fatalf("rows = %v, want %v", ids, want)
	}
	cur := byID["claude-opus-4-1"]
	if !cur.Current || cur.Family != "opus" || cur.Version == nil || *cur.Version != (wire.ModelVersion{Major: 4, Minor: 1}) ||
		cur.Latest != "claude-opus-4-5" || cur.SwitchEffort != "high" || fmt.Sprint(cur.Efforts) != "[low medium high]" {
		t.Errorf("current row = %+v", cur)
	}
	if next := byID["claude-opus-4-5"]; next.Current || next.SwitchEffort != "medium" || next.Latest != "" {
		t.Errorf("newer row = %+v, want effort lowered to medium and no newer model", next)
	}
	if odd := byID["custom"]; odd.Family != "" || odd.Version != nil || odd.Efforts == nil || len(odd.Efforts) != 0 || odd.SwitchEffort != "" {
		t.Errorf("unparseable row = %+v", odd)
	}
	if len(res.Aliases) != 1 || res.Aliases[0] != (wire.ModelAlias{Alias: "opus", ID: "claude-opus-4-1", Latest: "claude-opus-4-5"}) {
		t.Errorf("aliases = %+v", res.Aliases)
	}
}

func TestWebSessions_ModelsKeepsTheCurrentModelWithoutAList(t *testing.T) {
	t.Parallel()
	f, _ := modelsServer(t, nil, nil)
	seedModelSession(t, f, "s1", "claude", "claude-opus-4-1", "high")
	code, res, body := models(t, f, "s1")
	if code != http.StatusOK || len(res.Models) != 1 || res.Models[0].ID != "claude-opus-4-1" || !res.Models[0].Current || len(res.Models[0].Efforts) != 0 {
		t.Fatalf("models = %d %s, want the current model as the only row with no efforts", code, body)
	}

	f2, _ := modelsServer(t, modelsClaude, nil)
	seedModelSession(t, f2, "s2", "claude", "claude-opus-9-9", "high")
	res = modelsOnceListed(t, f2, "s2", 4)
	last := res.Models[len(res.Models)-1]
	if last.ID != "claude-opus-9-9" || !last.Current || len(last.Efforts) != 0 {
		t.Errorf("rows = %+v, want the unlisted current model last with no efforts", res.Models)
	}
}

func TestWebSessions_ModelsRefreshesOnceAndAnswersWithoutWaiting(t *testing.T) {
	t.Parallel()
	block := make(chan struct{})
	f, calls := modelsServer(t, modelsClaude, block)
	seedModelSession(t, f, "s1", "claude", "opus", "high")

	// The server's own start-up read is the refresh in flight.
	if !testutil.Poll(10*time.Second, 5*time.Millisecond, func() bool { return calls.Load() == 1 }) {
		t.Fatal("the start-up read never began")
	}
	for range 3 {
		code, res, body := models(t, f, "s1")
		if code != http.StatusOK || !res.Refreshing || len(res.Models) != 1 {
			t.Fatalf("while the probe runs: %d %s, want the current row and refreshing", code, body)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("probes = %d while one was running, want 1", n)
	}
	close(block)
	modelsOnceListed(t, f, "s1", 3)
	if !testutil.Poll(10*time.Second, 10*time.Millisecond, func() bool {
		_, res, _ := models(t, f, "s1")
		return len(res.Models) == 3 && calls.Load() >= 2
	}) {
		t.Errorf("a call after the first read started no new refresh: %d probes", calls.Load())
	}
}

func TestWebSessions_ModelsRefusals(t *testing.T) {
	t.Parallel()
	f, _ := modelsServer(t, modelsClaude, nil)
	seedModelSession(t, f, "other", "no-such-harness", "x", "")
	for name, tc := range map[string]struct {
		id     string
		code   int
		reason string
	}{
		"unknown session":    {"nope", http.StatusNotFound, wire.ReasonSessionUnknown},
		"queue agent":        {queue.AgentIDPrefix + "1", http.StatusNotFound, wire.ReasonSessionUnknown},
		"unsupported":        {"other", http.StatusBadRequest, wire.ReasonPromptHarnessNotSupported},
		"bad id":             {"../x", http.StatusBadRequest, wire.ReasonSessionBadRequest},
		"missing session id": {"", http.StatusBadRequest, wire.ReasonSessionBadRequest},
	} {
		code, _, body := models(t, f, tc.id)
		if code != tc.code || servertest.ReasonOf(t, body) != tc.reason {
			t.Errorf("%s: %d %s, want %d %s", name, code, body, tc.code, tc.reason)
		}
	}
}
