package settingsapi_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire/ops"
)

type apiFixture struct {
	h      *servertest.Harness
	store  string
	config string
}

func startAPI(t *testing.T, fields map[string]any) apiFixture {
	t.Helper()
	store := t.TempDir()
	if err := servertest.WriteEpic(store, "proj", "epic"); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", t.TempDir())
	path := filepath.Join(t.TempDir(), "config.json")
	if ts, ok := fields["ticket-store"].(map[string]any); ok {
		ts["path"] = store
	}
	servertest.WriteConfig(t, path, store, fields)
	return apiFixture{h: servertest.StartWithStore(t, store, servertest.WithConfigFile(path)), store: store, config: path}
}

func (f apiFixture) set(t *testing.T, project string, changes ...ops.SettingChange) ops.SettingsWriteResult {
	t.Helper()
	res, err := f.h.Client.Admin.SetSettings(context.Background(), ops.SettingsWriteRequest{Project: project, Changes: changes})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (f apiFixture) read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// queueConfig reads the blocks these tests check; the file also holds
// top-level keys that are not objects.
type queueConfig struct {
	Queue  map[string]any `json:"execution-queue"`
	Budget map[string]any `json:"budget"`
}

func val(key, v string) ops.SettingChange {
	return ops.SettingChange{Key: key, Value: json.RawMessage(v)}
}

func remove(key string) ops.SettingChange { return ops.SettingChange{Key: key, Remove: true} }

func TestWriteSetAndRemoveKeepOtherKeys(t *testing.T) {
	f := startAPI(t, map[string]any{"execution-queue": map[string]any{"max-agents": 2, "max-one-off-agents": 4}, "budget": map[string]any{"soft-limit": 10}})
	res := f.set(t, "", val("execution-queue.max-agents", "5"))
	if res.Refused || string(res.Values["execution-queue.max-agents"]) != "5" {
		t.Fatalf("result = %+v", res)
	}
	var cfg queueConfig
	if err := json.Unmarshal([]byte(f.read(t, f.config)), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Queue["max-agents"] != float64(5) || cfg.Queue["max-one-off-agents"] != float64(4) || cfg.Budget["soft-limit"] != float64(10) {
		t.Fatalf("config = %v", cfg)
	}
	res = f.set(t, "", remove("execution-queue.max-agents"))
	if res.Refused || res.Values["execution-queue.max-agents"] != nil && string(res.Values["execution-queue.max-agents"]) != "null" {
		t.Fatalf("remove result = %+v", res)
	}
	if got := f.read(t, f.config); strings.Contains(got, `"max-agents"`) || !strings.Contains(got, `"max-one-off-agents"`) {
		t.Fatalf("after remove: %s", got)
	}
}

func TestWriteRefusalNamesKeyAndWritesNothing(t *testing.T) {
	f := startAPI(t, nil)
	before := f.read(t, f.config)
	res := f.set(t, "", val("execution-queue.max-agents", `"x"`))
	if !res.Refused || len(res.Keys) != 1 || res.Keys[0] != "execution-queue.max-agents" {
		t.Fatalf("result = %+v", res)
	}
	res = f.set(t, "", val("execution-queue.max-agents", "7"), val("execution-queue.max-one-off-agents", `"x"`))
	if !res.Refused || res.Keys[0] != "execution-queue.max-one-off-agents" {
		t.Fatalf("two-key result = %+v", res)
	}
	if got := f.read(t, f.config); got != before {
		t.Fatalf("file changed:\n%s\nwas:\n%s", got, before)
	}
}

func TestWriteSpinWindowDuration(t *testing.T) {
	f := startAPI(t, nil)
	if res := f.set(t, "", val("execution-queue.waiting-slot-grace", `"5mm"`)); !res.Refused {
		t.Fatalf("5mm saved: %+v", res)
	}
	if res := f.set(t, "", val("execution-queue.waiting-slot-grace", `"5m"`)); res.Refused {
		t.Fatalf("5m refused: %+v", res)
	}
}

func TestWriteRestartNeeded(t *testing.T) {
	f := startAPI(t, nil)
	res := f.set(t, "", val("server.web.listen", `"127.0.0.1:0"`))
	if len(res.RestartNeeded) != 1 || res.RestartNeeded[0] != "server.web.listen" {
		t.Fatalf("restart = %+v", res)
	}
	res = f.set(t, "", val("execution-queue.max-agents", "3"))
	if len(res.RestartNeeded) != 0 {
		t.Fatalf("live key reported restart: %+v", res)
	}
}

func TestWriteWithWatcherReloadsOnceWithoutBanner(t *testing.T) {
	f := startAPI(t, map[string]any{"server": map[string]any{"watch-config": true}})
	f.set(t, "", val("execution-queue.max-agents", "6"))
	// Watch the watcher's would-be reaction window: a banner, or a last reload
	// that is no longer the web one, means it reloaded a second time.
	if servertest.Poll(1500*time.Millisecond, func() bool {
		st, err := f.h.Client.Admin.ConfigStatus(context.Background())
		return err != nil || len(f.h.Banners.Sent()) != 0 || st.LastReload == nil || st.LastReload.Trigger != "web"
	}) {
		t.Fatal("the save did not reload exactly once, without a banner")
	}
}

func TestWriteConcurrentSavesAreBothKept(t *testing.T) {
	f := startAPI(t, nil)
	var wg sync.WaitGroup
	for _, c := range []ops.SettingChange{val("execution-queue.max-agents", "4"), val("execution-queue.max-one-off-agents", "5")} {
		wg.Go(func() { f.set(t, "", c) })
	}
	wg.Wait()
	got := f.read(t, f.config)
	if !strings.Contains(got, `"max-agents": 4`) && !strings.Contains(got, `"max-agents":4`) || !strings.Contains(got, "5") {
		t.Fatalf("config = %s", got)
	}
	var cfg queueConfig
	if err := json.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Queue["max-agents"] != float64(4) || cfg.Queue["max-one-off-agents"] != float64(5) {
		t.Fatalf("config = %v", cfg)
	}
}

func TestWriteProjectLandsInProjectJSONAndIsCommitted(t *testing.T) {
	f := startAPI(t, map[string]any{"ticket-store": map[string]any{"commit-debounce": 1}})
	res := f.set(t, "proj", val("project.max-agents", "2"))
	if res.Refused {
		t.Fatalf("result = %+v", res)
	}
	file := filepath.Join(f.store, "proj", "project.json")
	if got := f.read(t, file); !strings.Contains(got, `"max-agents"`) {
		t.Fatalf("project.json = %s", got)
	}
	servertest.WaitFor(t, 15*time.Second, "the store commit of project.json", func() bool {
		out, err := exec.Command("git", "-C", f.store, "show", "HEAD:proj/project.json").Output()
		return err == nil && strings.Contains(string(out), `"max-agents"`)
	})
}

func TestWriteProjectRefusesNotificationsAndKeepsHandWrittenBlock(t *testing.T) {
	f := startAPI(t, nil)
	file := filepath.Join(f.store, "proj", "project.json")
	raw := f.read(t, file)
	handWritten := strings.TrimSuffix(strings.TrimSpace(raw), "}") + `,"notifications":{"telegram":{"chat-id":"42"}}}`
	if err := os.WriteFile(file, []byte(handWritten), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"notifications.telegram.chat-id", "evolve.deep-read-cap"} {
		res := f.set(t, "proj", val(key, `"1"`))
		if !res.Refused || len(res.Keys) != 1 || res.Keys[0] != key {
			t.Fatalf("%s: result = %+v", key, res)
		}
	}
	if res := f.set(t, "proj", val("project.max-agents", "2")); res.Refused {
		t.Fatalf("max-agents refused: %+v", res)
	}
	var pf struct {
		Notifications map[string]map[string]string `json:"notifications"`
	}
	if err := json.Unmarshal([]byte(f.read(t, file)), &pf); err != nil {
		t.Fatal(err)
	}
	if pf.Notifications["telegram"]["chat-id"] != "42" {
		t.Fatalf("notifications block lost: %s", f.read(t, file))
	}
}

// Seam: a web save of the global Default harness reloads once with the web
// trigger, so no OS banner pops (the command trigger would).
func TestWriteDefaultHarnessGlobalPopsNoBanner(t *testing.T) {
	f := startAPI(t, map[string]any{})
	res := f.set(t, "", val("agents.default-harness", `"codex"`))
	if res.Refused {
		t.Fatalf("result = %+v", res)
	}
	if n := len(f.h.Banners.Sent()); n != 0 {
		t.Errorf("web save sent %d banners, want none", n)
	}
}
