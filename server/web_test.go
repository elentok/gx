package server_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func startWeb(t *testing.T) (*servertest.Harness, string) {
	t.Helper()
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if hs.WebURL == "" {
		t.Fatalf("handshake has no web_url: %+v", hs)
	}
	return h, hs.WebURL
}

func webDo(t *testing.T, method, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestWeb_OffByDefault(t *testing.T) {
	hs, err := servertest.Start(t).Client.Handshake(context.Background())
	if err != nil || hs.WebURL != "" || hs.WebError != "" {
		t.Errorf("handshake web fields = %q %q (err %v), want empty", hs.WebURL, hs.WebError, err)
	}
}

func TestWeb_ServesPlaceholderAtRootAndFallback(t *testing.T) {
	_, base := startWeb(t)
	for _, path := range []string{"/", "/index.html", "/some/client/route"} {
		code, body := webDo(t, http.MethodGet, base+path)
		if code != http.StatusOK || !strings.Contains(body, "This build has no web UI") {
			t.Errorf("GET %s = %d %q", path, code, body)
		}
	}
}

func TestWeb_SnapshotIsServed(t *testing.T) {
	_, base := startWeb(t)
	if code, _ := webDo(t, http.MethodGet, base+"/v1/snapshot"); code != http.StatusOK {
		t.Errorf("GET /v1/snapshot = %d", code)
	}
}

func TestWeb_EverythingElseIs404(t *testing.T) {
	_, base := startWeb(t)
	for _, pattern := range server.RoutePatterns() {
		method, path, _ := strings.Cut(pattern, " ")
		if pattern == "GET /v1/snapshot" || pattern == "GET /v1/events" {
			continue
		}
		if code, _ := webDo(t, method, base+path); code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", pattern, code)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		for _, path := range []string{"/v1/snapshot", "/v1/events", "/"} {
			if code, _ := webDo(t, method, base+path); code != http.StatusNotFound {
				t.Errorf("%s %s = %d, want 404", method, path, code)
			}
		}
	}
}

func TestWeb_BindFailureIsReportedAndServerKeepsRunning(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { taken.Close() })
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) { c.WebAddr = taken.Addr().String() })

	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if hs.WebURL != "" || hs.WebError == "" {
		t.Errorf("handshake web fields = %q %q, want an error and no url", hs.WebURL, hs.WebError)
	}
}
