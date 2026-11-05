package servertest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server"
)

const (
	EpicMergedText    = "epic merged"
	EpicNotMergedText = "epic complete (not merged)"
)

// ChatRecorder is a server option sending chat to a recording webhook, and a
// count of how often s was sent.
func ChatRecorder(t *testing.T) (opt func(*server.Config), count func(s string) int) {
	t.Helper()
	var mu sync.Mutex
	var sent strings.Builder
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		sent.Write(b)
		mu.Unlock()
	}))
	t.Cleanup(hook.Close)
	opt = func(c *server.Config) {
		c.Chat = notify.ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: FlushInterval}
	}
	count = func(s string) int {
		mu.Lock()
		defer mu.Unlock()
		return strings.Count(sent.String(), s)
	}
	return opt, count
}

// WaitForChat polls until s was sent, and reports how often.
func WaitForChat(count func(string) int, s string) int {
	Poll(10*time.Second, func() bool { return count(s) > 0 })
	return count(s)
}
