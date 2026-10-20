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

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/testutil/herdrfake"
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
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
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
	fake := herdrfake.NewState(t)
	fake.Register("workspace", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspaces": []any{}}, herdrfake.Identities{}, nil
	})
	herdrfake.StartState(t, fake)
	stateDir, err := os.MkdirTemp("", "gx") // short path: unix socket limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(Config{StateDir: stateDir, TicketStore: store, ConfigPath: path, Settings: Settings{
		Chat: ralphloop.ServerChatConfig{SlackWebhookURL: global, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: flush},
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
	s.chat.Notice(ralphloop.ServerNotice{Kind: kind, Emoji: "🔔", Title: kind})
}

func TestConfigReloadChat_ChangedDestinationTakesLaterMessages(t *testing.T) {
	oldHook, newHook := newChatHook(t, nil), newChatHook(t, nil)
	s, path := reloadChatServer(t, oldHook.URL, 20*time.Millisecond)
	defer s.chat.Close()

	writeChatConfig(t, s, path, newHook.URL, 3)
	if res := s.reloadConfig(reloadTriggerCommand); !slices.Contains(res.Changed, "notifications.slack.webhook-url") {
		t.Fatalf("reload = %+v, want webhook-url changed", res)
	}
	notice(s, "after-reload")
	newHook.waitFor(t, "after-reload")
	if got := oldHook.sent(); len(got) != 0 {
		t.Errorf("old destination got %v, want nothing", got)
	}
}

func TestConfigReloadChat_RemovedCredentialsStopMessages(t *testing.T) {
	hook := newChatHook(t, nil)
	s, path := reloadChatServer(t, hook.URL, 20*time.Millisecond)
	defer s.chat.Close()

	writeChatConfig(t, s, path, "", 3)
	s.reloadConfig(reloadTriggerCommand)
	notice(s, "after-reload")
	time.Sleep(200 * time.Millisecond)
	if got := hook.sent(); len(got) != 0 || s.chat.Enabled() {
		t.Errorf("sent %v (enabled %v), want nothing", got, s.chat.Enabled())
	}
}

// One batch per destination: messages queued before and after a reload that
// keeps the destination's sink arrive together on Close.
func TestConfigReloadChat_UnchangedChatKeepsTheBatch(t *testing.T) {
	hook := newChatHook(t, nil)
	s, path := reloadChatServer(t, hook.URL, time.Hour)

	notice(s, "before-reload")
	writeChatConfig(t, s, path, hook.URL, 4)
	if res := s.reloadConfig(reloadTriggerCommand); slices.Contains(res.Changed, "notifications.slack.webhook-url") {
		t.Fatalf("reload = %+v, want chat unchanged", res)
	}
	notice(s, "after-reload")
	s.chat.Close()
	if got := hook.sent(); len(got) != 1 || !strings.Contains(got[0], "before-reload") || !strings.Contains(got[0], "after-reload") {
		t.Errorf("sent %v, want one batch with both notices", got)
	}
}

func TestConfigReloadChat_ProjectSinkKeepsItsBatch(t *testing.T) {
	oldHook, newHook, projectHook := newChatHook(t, nil), newChatHook(t, nil), newChatHook(t, nil)
	s, path := reloadChatServer(t, oldHook.URL, time.Hour)
	override := &ralphloop.ServerChatConfig{SlackWebhookURL: projectHook.URL}

	s.chat.Park("proj", override, "epic", "/t/01.md", "01", "needs-answer", "zero-commit", "before-reload", ralphloop.EpicCounts{})
	writeChatConfig(t, s, path, newHook.URL, 3)
	s.reloadConfig(reloadTriggerCommand)
	s.chat.Park("proj", override, "epic", "/t/02.md", "02", "needs-answer", "zero-commit", "after-reload", ralphloop.EpicCounts{})
	s.chat.Close()
	if got := projectHook.sent(); len(got) != 1 || !strings.Contains(got[0], "before-reload") || !strings.Contains(got[0], "after-reload") {
		t.Errorf("project sent %v, want one batch with both parks", got)
	}
}

func TestConfigReloadChat_ReturnsBeforeOldSinkCloses(t *testing.T) {
	gate := make(chan struct{})
	oldHook, newHook := newChatHook(t, gate), newChatHook(t, nil)
	s, path := reloadChatServer(t, oldHook.URL, time.Hour)
	defer s.chat.Close()

	notice(s, "queued")
	writeChatConfig(t, s, path, newHook.URL, 3)
	done := make(chan struct{})
	go func() {
		s.reloadConfig(reloadTriggerCommand)
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
