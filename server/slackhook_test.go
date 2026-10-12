package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// slackHook is a Slack webhook that records every post. The chat batches what
// arrives within one flush window into one post, so a test that filters posts
// by content can drop a message that shared a post with the start notice;
// waitStarted lets a test begin counting only once that notice has gone out.
type slackHook struct {
	*httptest.Server
	mu    sync.Mutex
	posts []string
}

func newSlackHook(t *testing.T) *slackHook {
	t.Helper()
	h := &slackHook{}
	h.Server = httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		h.mu.Lock()
		h.posts = append(h.posts, string(b))
		h.mu.Unlock()
	}))
	t.Cleanup(h.Close)
	return h
}

// waitStarted waits for the server's start notice and returns how many posts
// arrived up to it, to pass to postsSince.
func (h *slackHook) waitStarted(t *testing.T) int {
	t.Helper()
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		h.mu.Lock()
		i := slices.IndexFunc(h.posts, func(p string) bool { return strings.Contains(p, "server started") })
		h.mu.Unlock()
		if i >= 0 {
			return i + 1
		}
	}
	t.Fatal("no server started notice in chat")
	return 0
}

// postsSince waits until more than from posts have arrived, then a grace
// period for any extra one to show, and returns the posts after from.
func (h *slackHook) postsSince(from int) []string {
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		h.mu.Lock()
		got := len(h.posts)
		h.mu.Unlock()
		if got > from {
			break
		}
	}
	time.Sleep(time.Second)
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.posts[from:])
}
