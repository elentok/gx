package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// startWebDev starts a server whose web UI proxies to devURL. It returns the
// web base URL and the harness state dir, which holds the login token.
func startWebDev(t *testing.T, devURL string) (string, *servertest.Harness) {
	t.Helper()
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.WebDevURL = devURL
	})
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if hs.WebURL == "" {
		t.Fatalf("handshake has no web_url, web_error = %q", hs.WebError)
	}
	return hs.WebURL, h
}

func TestWebDevProxy_ServesAssetsFromDevFrontend(t *testing.T) {
	dev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("dev:" + r.URL.Path))
	}))
	t.Cleanup(dev.Close)

	base, h := startWebDev(t, dev.URL)
	cookie := login(t, h.StateDir, base)
	code, body := webDoCookie(t, http.MethodGet, base+"/src/main.tsx", cookie)
	if code != http.StatusOK || body != "dev:/src/main.tsx" {
		t.Errorf("asset = %d %q, want the dev frontend's response", code, body)
	}
}

func TestWebDevProxy_SnapshotStaysLocal(t *testing.T) {
	dev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("dev frontend got %s", r.URL.Path)
	}))
	t.Cleanup(dev.Close)

	base, h := startWebDev(t, dev.URL)
	cookie := login(t, h.StateDir, base)
	if code, _ := webDoCookie(t, http.MethodGet, base+"/v1/snapshot", cookie); code != http.StatusOK {
		t.Errorf("snapshot status = %d, want 200", code)
	}
}

func TestWebDevProxy_RejectsNonLoopbackTarget(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.WebDevURL = "http://example.com:5173"
	})
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if hs.WebURL != "" || hs.WebError == "" {
		t.Errorf("web_url = %q, web_error = %q; want the bind refused with an error", hs.WebURL, hs.WebError)
	}
}
