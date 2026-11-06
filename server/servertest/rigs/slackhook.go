package rigs

import (
	"github.com/elentok/gx/server/servertest"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// SlackHook is a Slack webhook that records every post. The chat batches what
// arrives within one flush window into one post, so a test that filters posts
// by content can drop a message that shared a post with the start notice;
// WaitStarted lets a test begin counting only once that notice has gone out.
type SlackHook struct {
	*httptest.Server
	mu    sync.Mutex
	posts []string
}

// NewSlackHook starts a SlackHook, closed when the test ends.
func NewSlackHook(t *testing.T) *SlackHook {
	t.Helper()
	h := &SlackHook{}
	h.Server = httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		h.mu.Lock()
		h.posts = append(h.posts, string(b))
		h.mu.Unlock()
	}))
	t.Cleanup(h.Close)
	return h
}

// WaitStarted waits for the server's start notice and returns how many posts
// arrived up to it, to pass to PostsSince.
func (h *SlackHook) WaitStarted(t *testing.T) int {
	t.Helper()
	var i int
	servertest.WaitFor(t, 15*time.Second, "the server started notice in chat", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		i = slices.IndexFunc(h.posts, func(p string) bool { return strings.Contains(p, "Server started") })
		return i >= 0
	})
	return i + 1
}

// PostsSince waits until more than from posts have arrived, then a grace
// period for any extra one to show, and returns the posts after from.
func (h *SlackHook) PostsSince(from int) []string {
	servertest.Poll(15*time.Second, func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return len(h.posts) > from
	})
	time.Sleep(500 * time.Millisecond) // negative wait: an extra post would land within a few flush windows
	return h.PostsFrom(from)
}

// PostsAfterAlert waits for the banner and a chat flush, then returns the posts after from.
func (h *SlackHook) PostsAfterAlert(hs *servertest.Harness, from int) []string {
	servertest.WaitAlerts(hs, 1)
	time.Sleep(5 * servertest.FlushInterval) // negative wait: no event marks the chat flush, so give it a few intervals
	return h.PostsFrom(from)
}

// PostsFrom returns the posts after from, without waiting.
func (h *SlackHook) PostsFrom(from int) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.posts[from:])
}
