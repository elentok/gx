package server

import (
	"encoding/json"
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

	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/ralphloop/schedule"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/testutil/runnerfake"
)

// chatHook is a fake Slack webhook that records each body. With a gate, every
// request waits for it to close.
type chatHook struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
	gate   chan struct{}
}

func newChatHook(t *testing.T, gate chan struct{}) *chatHook {
	t.Helper()
	h := &chatHook{gate: gate}
	h.Server = httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if h.gate != nil {
			<-h.gate
		}
		b, _ := io.ReadAll(r.Body)
		h.mu.Lock()
		defer h.mu.Unlock()
		h.bodies = append(h.bodies, string(b))
	}))
	t.Cleanup(h.Close)
	return h
}

func (h *chatHook) sent() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.bodies)
}

// waitFor returns the bodies once one contains want, or fails after a while.
func (h *chatHook) waitFor(t *testing.T, want string) []string {
	t.Helper()
	// This file is package server, which servertest imports, so it cannot use
	// servertest.WaitFor.
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if got := h.sent(); slices.ContainsFunc(got, func(b string) bool { return strings.Contains(b, want) }) {
			return got
		}
	}
	t.Fatalf("bodies = %v, want one containing %q", h.sent(), want)
	return nil
}

// reloadChatServer is a server notifying global, whose config file is at the
// returned path. flush is the batch tick.
func reloadChatServer(t *testing.T, global string, flush time.Duration) (*Server, string) {
	t.Helper()
	return reloadChatServerWith(t, notify.ServerChatConfig{SlackWebhookURL: global, FlushInterval: flush})
}

// reloadChatServerWith is reloadChatServer with the whole chat config given;
// the gate state path is filled in.
func reloadChatServerWith(t *testing.T, chat notify.ServerChatConfig) (*Server, string) {
	t.Helper()
	chat.GateStatePath = filepath.Join(t.TempDir(), "gate.json")
	stateDir, err := os.MkdirTemp("", "gx") // short path: unix socket limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: store, ConfigPath: path, Settings: Settings{
		Chat: chat,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func writeChatConfig(t *testing.T, s *Server, path, slack string, maxAgents int) {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"ticket-store":    map[string]any{"path": s.cfg.TicketStore},
		"execution-queue": map[string]any{"max-agents": maxAgents},
		"notifications":   map[string]any{"slack": map[string]any{"webhook-url": slack}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func notice(s *Server, kind string) {
	s.chat.Notice(notify.ServerNotice{Kind: kind, Emoji: "🔔", Title: kind, Detail: kind})
}

func TestConfigReloadChat_ChangedDestinationTakesLaterMessages(t *testing.T) {
	t.Parallel()
	oldHook, newHook := newChatHook(t, nil), newChatHook(t, nil)
	s, path := reloadChatServer(t, oldHook.URL, 20*time.Millisecond)
	defer s.chat.Close()

	writeChatConfig(t, s, path, newHook.URL, 3)
	if res := s.settings.Reload(settings.TriggerCommand); !slices.Contains(res.Changed, "notifications.slack.webhook-url") {
		t.Fatalf("reload = %+v, want webhook-url changed", res)
	}
	notice(s, "after-reload")
	newHook.waitFor(t, "after-reload")
	if got := oldHook.sent(); len(got) != 0 {
		t.Errorf("old destination got %v, want nothing", got)
	}
}

// writeSlackChannelConfig writes a config whose Slack channel has the given
// switch and kind list.
func writeSlackChannelConfig(t *testing.T, s *Server, path, webhook string, enabled bool, kinds []string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"ticket-store":          map[string]any{"path": s.cfg.TicketStore},
		"notifications":         map[string]any{"slack": map[string]any{"webhook-url": webhook}},
		"notification-channels": map[string]any{"slack": map[string]any{"enabled": enabled, "kinds": kinds}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// The Slack switch and kind list apply on the next send after a reload, with
// no restart. Each phase ends with a message that must arrive, so a message
// that must not arrive would sit in the same batch if it had been queued.
func TestConfigReloadChat_ChannelSwitchAndKindListApplyOnTheNextSend(t *testing.T) {
	t.Parallel()
	hook := newChatHook(t, nil)
	s, path := reloadChatServer(t, hook.URL, 20*time.Millisecond)
	defer s.chat.Close()
	reload := func(enabled bool, kinds []string) {
		t.Helper()
		writeSlackChannelConfig(t, s, path, hook.URL, enabled, kinds)
		s.settings.Reload(settings.TriggerCommand)
	}

	reload(true, []string{"disk"})
	notice(s, "server-started")
	notice(s, "disk-warn")
	hook.waitFor(t, "disk-warn")

	reload(false, []string{"disk"})
	notice(s, "disk-resume")
	reload(true, []string{"disk"})
	notice(s, "disk-clean")
	hook.waitFor(t, "disk-clean")

	for _, body := range hook.sent() {
		for _, banned := range []string{"server-started", "disk-resume"} {
			if strings.Contains(body, banned) {
				t.Errorf("body %q holds %q, which the kind list or the switch should have dropped", body, banned)
			}
		}
	}
}

// writeChannelSwitchConfig writes a config with every channel's switch set to
// on (the kind lists stay at their defaults).
func writeChannelSwitchConfig(t *testing.T, s *Server, path, webhook string, on bool) {
	t.Helper()
	sw := map[string]any{"enabled": on}
	data, err := json.Marshal(map[string]any{
		"ticket-store":  map[string]any{"path": s.cfg.TicketStore},
		"notifications": map[string]any{"slack": map[string]any{"webhook-url": webhook}},
		"notification-channels": map[string]any{
			"desktop": sw, "web-toast": sw, "telegram": sw, "slack": sw, "push": sw,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A changed switch reaches every channel's reader on the next reload: the
// router reads settings.Settings per message, nothing is captured at start.
func TestConfigReloadChannelSwitch_AppliesWithoutRestart(t *testing.T) {
	t.Parallel()
	s, path := reloadChatServer(t, "http://127.0.0.1:1/hook", time.Hour)
	defer s.chat.Close()
	outcomes := func() map[string]string {
		return map[string]string{
			"desktop":   preflight(s, notify.ChannelDesktop, func(n settings.NotifyChannels) settings.NotifyChannel { return n.Desktop }, notifykind.SessionAlert, true),
			"web-toast": preflight(s, notify.ChannelWebToast, func(n settings.NotifyChannels) settings.NotifyChannel { return n.WebToast }, notifykind.SessionAlert, true),
			"push":      preflight(s, notify.ChannelPush, func(n settings.NotifyChannels) settings.NotifyChannel { return n.Push }, notifykind.SessionAlert, true),
		}
	}

	writeChannelSwitchConfig(t, s, path, "http://127.0.0.1:1/hook", false)
	res := s.settings.Reload(settings.TriggerCommand)
	for _, key := range []string{"desktop", "web-toast", "telegram", "slack", "push"} {
		if !slices.Contains(res.Changed, "notification-channels."+key+".enabled") {
			t.Errorf("reload changed %v, want the %s switch", res.Changed, key)
		}
	}
	for name, got := range outcomes() {
		if got != notify.OutcomeOff {
			t.Errorf("%s after switching off = %q, want %q", name, got, notify.OutcomeOff)
		}
	}
	if s.chat.Enabled() {
		t.Error("chat still enabled with Telegram and Slack off")
	}

	writeChannelSwitchConfig(t, s, path, "http://127.0.0.1:1/hook", true)
	s.settings.Reload(settings.TriggerCommand)
	for name, got := range outcomes() {
		if got != notify.OutcomeSent {
			t.Errorf("%s after switching on = %q, want %q", name, got, notify.OutcomeSent)
		}
	}
	if !s.chat.Enabled() {
		t.Error("chat not enabled after switching back on")
	}
}

// A notice queued while the switch was on is dropped when the switch is off at
// the flush; Desktop, Web toast and Push queue nothing, so only chat has this.
func TestConfigReloadChat_QueuedBatchForATurnedOffChannelIsDropped(t *testing.T) {
	t.Parallel()
	hook := newChatHook(t, nil)
	s, path := reloadChatServer(t, hook.URL, time.Hour)

	notice(s, "queued-before-off")
	writeSlackChannelConfig(t, s, path, hook.URL, false, notifykind.IDs())
	s.settings.Reload(settings.TriggerCommand)
	s.chat.Close()
	if got := hook.sent(); len(got) != 0 {
		t.Errorf("sent %v, want nothing for a channel turned off", got)
	}
}

func TestConfigReloadChat_RemovedCredentialsStopMessages(t *testing.T) {
	t.Parallel()
	hook := newChatHook(t, nil)
	s, path := reloadChatServer(t, hook.URL, 20*time.Millisecond)
	defer s.chat.Close()

	writeChatConfig(t, s, path, "", 3)
	s.settings.Reload(settings.TriggerCommand)
	notice(s, "after-reload")
	time.Sleep(200 * time.Millisecond) // negative wait: a send would come within a few 50ms chat polls
	if got := hook.sent(); len(got) != 0 || s.chat.Enabled() {
		t.Errorf("sent %v (enabled %v), want nothing", got, s.chat.Enabled())
	}
}

// One batch per destination: messages queued before and after a reload that
// keeps the destination's sink arrive together on Close.
func TestConfigReloadChat_UnchangedChatKeepsTheBatch(t *testing.T) {
	t.Parallel()
	hook := newChatHook(t, nil)
	s, path := reloadChatServer(t, hook.URL, time.Hour)

	notice(s, "before-reload")
	writeChatConfig(t, s, path, hook.URL, 4)
	if res := s.settings.Reload(settings.TriggerCommand); slices.Contains(res.Changed, "notifications.slack.webhook-url") {
		t.Fatalf("reload = %+v, want chat unchanged", res)
	}
	notice(s, "after-reload")
	s.chat.Close()
	if got := hook.sent(); len(got) != 1 || !strings.Contains(got[0], "before-reload") || !strings.Contains(got[0], "after-reload") {
		t.Errorf("sent %v, want one batch with both notices", got)
	}
}

func TestConfigReloadChat_KeepsTheTestSeams(t *testing.T) {
	t.Parallel()
	s, path := reloadChatServerWith(t, notify.ServerChatConfig{
		SlackWebhookURL: "http://127.0.0.1:1/old", FlushInterval: time.Hour,
		ProgressInterval: 3 * time.Second, TelegramBaseURL: "http://127.0.0.1:2",
	})
	defer s.chat.Close()

	writeChatConfig(t, s, path, "http://127.0.0.1:1/new", 3)
	s.settings.Reload(settings.TriggerCommand)
	got := s.settings.Load().Chat
	if got.ProgressInterval != 3*time.Second || got.FlushInterval != time.Hour || got.TelegramBaseURL != "http://127.0.0.1:2" {
		t.Errorf("chat after reload = %+v, want the seams kept", got)
	}
}

func TestConfigReloadChat_ProjectSinkKeepsItsBatch(t *testing.T) {
	t.Parallel()
	oldHook, newHook, projectHook := newChatHook(t, nil), newChatHook(t, nil), newChatHook(t, nil)
	s, path := reloadChatServer(t, oldHook.URL, time.Hour)
	override := &notify.ServerChatConfig{SlackWebhookURL: projectHook.URL}

	s.chat.Pause("proj", override, "epic", "/t/01.md", "01", "paused", "zero-commit", "before-reload", schedule.EpicCounts{})
	writeChatConfig(t, s, path, newHook.URL, 3)
	s.settings.Reload(settings.TriggerCommand)
	s.chat.Pause("proj", override, "epic", "/t/02.md", "02", "paused", "zero-commit", "after-reload", schedule.EpicCounts{})
	s.chat.Close()
	if got := projectHook.sent(); len(got) != 1 || !strings.Contains(got[0], "before-reload") || !strings.Contains(got[0], "after-reload") {
		t.Errorf("project sent %v, want one batch with both pauses", got)
	}
}

func TestConfigReloadChat_ReturnsBeforeOldSinkCloses(t *testing.T) {
	t.Parallel()
	gate := make(chan struct{})
	oldHook, newHook := newChatHook(t, gate), newChatHook(t, nil)
	s, path := reloadChatServer(t, oldHook.URL, time.Hour)
	defer s.chat.Close()

	notice(s, "queued")
	writeChatConfig(t, s, path, newHook.URL, 3)
	done := make(chan struct{})
	go func() {
		s.settings.Reload(settings.TriggerCommand)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reload waited on the old sink's Close")
	}
	close(gate)
	oldHook.waitFor(t, "queued")
}
