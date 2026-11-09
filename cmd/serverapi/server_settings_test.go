package serverapi

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

const secretToken = "123456:SECRET-TOKEN-VALUE"

func TestServerSettings_JSONMatchesEndpointAndHidesSecrets(t *testing.T) {
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := servertest.WriteEpic(store, "proj", "epic"); err != nil {
		t.Fatal(err)
	}
	servertest.WriteConfig(t, path, store, map[string]any{
		"notifications": map[string]any{"telegram": map[string]any{"bot-token": secretToken}},
	})
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	ctx := context.Background()

	for _, target := range []string{"global", "proj"} {
		want, err := h.Client.Admin.Settings(ctx, target)
		if err != nil {
			t.Fatalf("%s endpoint: %v", target, err)
		}
		var out bytes.Buffer
		if err := runServerSettings(ctx, h.Client, &out, target, true); err != nil {
			t.Fatalf("%s cli: %v", target, err)
		}
		if strings.Contains(out.String(), secretToken) {
			t.Errorf("%s output leaks the secret", target)
		}
		var got wire.SettingsReadResponse
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("%s cli json: %v", target, err)
		}
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("%s: cli = %s, endpoint = %s", target, gotJSON, wantJSON)
		}
		keys := map[string]bool{}
		for _, e := range got.Entries {
			keys[e.Key] = true
		}
		for _, e := range registry.Default().All() {
			if target == "global" && !e.ProjectOnly && !keys[e.Key.String()] {
				t.Errorf("global read misses %s", e.Key)
			}
			if target == "proj" && e.ProjectOnly && !keys[e.Key.String()] {
				t.Errorf("project read misses %s", e.Key)
			}
			if target == "proj" && !e.ProjectOnly && !e.ProjectOverridable && len(e.GlobalKey) == 0 && keys[e.Key.String()] {
				t.Errorf("project read lists global-only key %s", e.Key)
			}
		}
	}
}

func TestServerSettings_UnknownProjectIsRefused(t *testing.T) {
	store := t.TempDir()
	h := servertest.StartWithStore(t, store)
	var out bytes.Buffer
	err := runServerSettings(context.Background(), h.Client, &out, "nope", true)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("err = %v, want an unknown project error", err)
	}
}

func TestServerSettingsSet_ResultMatchesEndpoint(t *testing.T) {
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := servertest.WriteEpic(store, "proj", "epic"); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", t.TempDir())
	servertest.WriteConfig(t, path, store, map[string]any{"execution-queue": map[string]any{"max-agents": 2}})
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	ctx := context.Background()

	run := func(req wire.SettingsWriteRequest) wire.SettingsWriteResult {
		t.Helper()
		var out bytes.Buffer
		if err := runServerSettingsSet(ctx, h.Client, &out, req, true); err != nil {
			t.Fatal(err)
		}
		var got wire.SettingsWriteResult
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("cli json %s: %v", out.String(), err)
		}
		want, err := h.Client.Admin.SetSettings(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("cli = %s, endpoint = %s", gotJSON, wantJSON)
		}
		return got
	}

	if res := run(wire.SettingsWriteRequest{Changes: []wire.SettingChange{{Key: "execution-queue.max-agents", Value: json.RawMessage("5")}}}); res.Refused {
		t.Fatalf("set refused: %+v", res)
	}
	if res := run(wire.SettingsWriteRequest{Changes: []wire.SettingChange{{Key: "execution-queue.max-agents", Remove: true}}}); res.Refused {
		t.Fatalf("remove refused: %+v", res)
	}
	if res := run(wire.SettingsWriteRequest{Project: "proj", Changes: []wire.SettingChange{{Key: "project.max-agents", Value: json.RawMessage("3")}}}); res.Refused {
		t.Fatalf("project set refused: %+v", res)
	}
	if b, err := os.ReadFile(filepath.Join(store, "proj", "project.json")); err != nil || !strings.Contains(string(b), `"max-agents": 3`) {
		t.Fatalf("project.json = %s, %v", b, err)
	}
	res := run(wire.SettingsWriteRequest{Changes: []wire.SettingChange{{Key: "execution-queue.max-agents", Value: json.RawMessage(`"x"`)}}})
	if !res.Refused || len(res.Keys) != 1 || res.Keys[0] != "execution-queue.max-agents" {
		t.Fatalf("bad value result = %+v", res)
	}
}

func TestServerSettingsSet_FlagsBuildChanges(t *testing.T) {
	if got := string(jsonValue("5")); got != "5" {
		t.Errorf("number = %s", got)
	}
	if got := string(jsonValue("5m")); got != `"5m"` {
		t.Errorf("duration = %s", got)
	}
	if got := string(jsonValue(`"3"`)); got != `"3"` {
		t.Errorf("quoted = %s", got)
	}
}
