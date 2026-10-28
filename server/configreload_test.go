package server_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

func reload(t *testing.T, h *servertest.Harness) server.ReloadResult {
	t.Helper()
	res, err := h.Client.ReloadConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func maxAgents(n int) map[string]any {
	return map[string]any{"execution-queue": map[string]any{"max-agents": n}}
}

// Seam A: a raised cap starts a queued root without waiting for the poll
// tick; a lowered one kills no running agent.
func TestConfigReload_MaxAgentsAppliesWithoutKillingAgents(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) {
		c.MaxAgents = 1
		c.PollInterval = time.Hour
		c.Sleep = time.Sleep
	})
	registerLaunch(h)
	// Started agents keep their slot: an iteration's finish wait never returns.
	idle := map[string]any{"agent": map[string]any{"pane_id": "p1", "agent_status": "idle", "state_change_seq": 1}}
	h.Herdr.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		if slices.Contains(argv, "blocked") {
			return nil, herdrfake.Identities{}, fmt.Errorf("timed out waiting for agent status")
		}
		return idle, herdrfake.Identities{}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, addr := range []string{"proj:epic-a/01", "proj:epic-b/01"} {
		if res, err := h.Client.QueueAdd(ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
	waitRuns(ctx, t, h, 1)

	servertest.WriteConfig(t, path, store, maxAgents(2))
	if res := reload(t, h); res.Refused || !slices.Contains(res.Changed, "execution-queue.max-agents") {
		t.Fatalf("raise = %+v, want max-agents changed", res)
	}
	waitRuns(ctx, t, h, 2)

	servertest.WriteConfig(t, path, store, maxAgents(1))
	if res := reload(t, h); res.Refused {
		t.Fatalf("lower = %+v", res)
	}
	time.Sleep(100 * time.Millisecond)
	if runs := h.Server.Runs(); len(runs) != 2 {
		t.Errorf("runs after lowering = %+v, want both agents still running", runs)
	}
}

func TestConfigReload_ReportsMaxOneOffAgentsChange(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	servertest.WriteConfig(t, path, store, map[string]any{"execution-queue": map[string]any{"max-one-off-agents": 7}})
	if res := reload(t, h); res.Refused || !slices.Contains(res.Changed, "execution-queue.max-one-off-agents") {
		t.Fatalf("reload = %+v, want max-one-off-agents changed", res)
	}
}

func waitRuns(ctx context.Context, t *testing.T, h *servertest.Harness, n int) {
	t.Helper()
	for len(h.Server.Runs()) != n {
		if ctx.Err() != nil {
			t.Fatalf("runs = %+v, want %d", h.Server.Runs(), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Seam A: an invalid or missing file is refused and leaves the live settings
// as they were.
func TestConfigReload_RefusesInvalidAndMissingFiles(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	servertest.WriteConfig(t, path, store, maxAgents(3))
	if res := reload(t, h); res.Refused {
		t.Fatalf("baseline = %+v", res)
	}

	if err := os.WriteFile(path, []byte(`{"execution-queue": {"max-agents": 5}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if res := reload(t, h); res.Reason != server.ReasonInvalidConfig || res.Message == "" {
		t.Errorf("bad json = %+v, want %s with the parse error", res, server.ReasonInvalidConfig)
	}
	servertest.WriteConfig(t, path, store, map[string]any{"execution-queue": map[string]any{"max-agents": 5}, "agent-runner": "tmux"})
	if res := reload(t, h); res.Reason != server.ReasonInvalidConfig {
		t.Errorf("bad agent-runner = %+v, want %s", res, server.ReasonInvalidConfig)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if res := reload(t, h); res.Reason != server.ReasonConfigMissing {
		t.Errorf("missing = %+v, want %s", res, server.ReasonConfigMissing)
	}

	servertest.WriteConfig(t, path, store, maxAgents(3))
	if res := reload(t, h); res.Refused || len(res.Changed) != 0 {
		t.Errorf("after refusals = %+v, want no changes", res)
	}
}

// Seam A: a startup-only key warns on every reload until restart, and the
// rest of the file still applies.
func TestConfigReload_StartupOnlyKeyWarnsUntilRestart(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) { c.MaxAgents = 3 })
	servertest.WriteConfig(t, path, store, map[string]any{
		"execution-queue": map[string]any{"max-agents": 4},
		"server":          map[string]any{"tcp-listen": true},
	})
	want := []string{"restart needed for server.tcp-listen"}
	res := reload(t, h)
	if res.Refused || !slices.Contains(res.Changed, "execution-queue.max-agents") || !slices.Equal(res.Warnings, want) {
		t.Fatalf("first = %+v, want max-agents changed and warnings %v", res, want)
	}
	if res := reload(t, h); slices.Contains(res.Changed, "execution-queue.max-agents") || !slices.Equal(res.Warnings, want) {
		t.Errorf("second = %+v, want no max-agents change and warnings %v", res, want)
	}
}

// Seam A: a server.web change warns and leaves the running listener alone.
func TestConfigReload_WebChangeWarnsAndKeepsListener(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	servertest.WriteConfig(t, path, store, map[string]any{
		"server": map[string]any{"web": map[string]any{"listen": "127.0.0.1:0"}},
	})
	res := reload(t, h)
	if want := []string{"restart needed for server.web.listen"}; res.Refused || !slices.Equal(res.Warnings, want) {
		t.Fatalf("reload = %+v, want warnings %v", res, want)
	}
	if h.Server.WebURL() != "" {
		t.Errorf("web listener = %q after reload, want unchanged (off)", h.Server.WebURL())
	}
}

// Seam A: every outcome is one banner and one config-reloaded event, and
// never a chat message.
func TestConfigReload_ReportsBannerAndEventNotChat(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var chat []string
	hook := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		chat = append(chat, string(b))
	}))
	t.Cleanup(hook.Close)
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) {
		c.MaxAgents = 3
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: time.Millisecond}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cases := []struct {
		name   string
		write  func()
		level  string
		banner osnotify.Notification
	}{
		{"success", func() { servertest.WriteConfig(t, path, store, maxAgents(4)) },
			server.EventLevelSuccess, osnotify.Notification{Title: "gx config reloaded successfully", Body: "changed: execution-queue.max-agents", Group: "gx-config"}},
		{"unchanged", func() {},
			server.EventLevelSuccess, osnotify.Notification{Title: "gx config reloaded successfully", Body: "no changes", Group: "gx-config"}},
		{"warning", func() {
			servertest.WriteConfig(t, path, store, map[string]any{"execution-queue": map[string]any{"max-agents": 4}, "server": map[string]any{"tcp-listen": true}})
		}, server.EventLevelWarning, osnotify.Notification{Title: "gx config reloaded with warnings", Body: "restart needed for server.tcp-listen", Group: "gx-config"}},
		{"invalid", func() {
			if err := os.WriteFile(path, []byte(`{`), 0o644); err != nil {
				t.Fatal(err)
			}
		}, server.EventLevelError, osnotify.Notification{Title: "gx config reload failed", Group: "gx-config"}},
	}
	// Baseline: the harness settings differ from any file in many keys.
	servertest.WriteConfig(t, path, store, maxAgents(3))
	reload(t, h)
	waitBanners(ctx, t, h, 1)
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	for i, tc := range cases {
		tc.write()
		reload(t, h)
		ev := servertest.WaitForEvent(ctx, t, evs, server.EventConfigReloaded, "")
		if ev.Level != tc.level || !strings.HasPrefix(ev.Message, tc.banner.Title) {
			t.Errorf("%s: event = %+v, want level %s and message from %q", tc.name, ev, tc.level, tc.banner.Title)
		}
		banners := waitBanners(ctx, t, h, i+2)
		got := banners[i+1]
		if tc.banner.Body == "" {
			got.Body = "" // the parse error's wording is the parser's
		}
		if got != tc.banner {
			t.Errorf("%s: banner = %+v, want %+v", tc.name, got, tc.banner)
		}
	}
	time.Sleep(100 * time.Millisecond)
	if n := len(h.Banners.Sent()); n != len(cases)+1 {
		t.Errorf("banners = %d, want one per reload (%d)", n, len(cases)+1)
	}
	mu.Lock()
	defer mu.Unlock()
	// The server's own "server started" message is not a reload's.
	for _, body := range chat {
		if strings.Contains(body, "config") {
			t.Errorf("chat send %q, want none for a reload", body)
		}
	}
}

func waitBanners(ctx context.Context, t *testing.T, h *servertest.Harness, n int) []osnotify.Notification {
	t.Helper()
	for {
		if got := h.Banners.Sent(); len(got) >= n {
			return got
		}
		if ctx.Err() != nil {
			t.Fatalf("banners = %+v, want %d", h.Banners.Sent(), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type failingNotifier struct{}

func (failingNotifier) Notify(context.Context, osnotify.Notification) error {
	return errors.New("terminal-notifier: exit status 1")
}

// Seam A: a banner that fails to send does not fail the reload.
func TestConfigReload_NotifierFailureDoesNotFailReload(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) { c.Notifier = failingNotifier{} })
	servertest.WriteConfig(t, path, store, maxAgents(4))
	if res := reload(t, h); res.Refused || !slices.Contains(res.Changed, "execution-queue.max-agents") {
		t.Errorf("reload = %+v, want max-agents changed", res)
	}
}

func TestConfigReload_UnavailableWithoutConfigPath(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)
	if res := reload(t, h); res.Reason != server.ReasonReloadUnavailable {
		t.Errorf("reload = %+v, want %s", res, server.ReasonReloadUnavailable)
	}
}
