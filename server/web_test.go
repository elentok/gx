package server_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
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

// noRedirect lets a test see the redirect itself instead of its target.
var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// login follows the token link the way a browser does and returns the cookie.
func login(t *testing.T, stateDir, base string) *http.Cookie {
	t.Helper()
	token, err := server.ReadWebToken(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := noRedirect.Get(base + "/?gx_token=" + token)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login set %d cookies, want 1 (status %d)", len(cookies), resp.StatusCode)
	}
	return cookies[0]
}

func webDoCookie(t *testing.T, method, url string, cookie *http.Cookie) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// webDo sends an authenticated request.
func webDo(t *testing.T, h *servertest.Harness, method, url string) (int, string) {
	t.Helper()
	base, _, _ := strings.Cut(url[len("http://"):], "/")
	return webDoCookie(t, method, url, login(t, h.StateDir, "http://"+base))
}

// webSend sends an authenticated request with the given headers and an empty JSON body.
func webSend(t *testing.T, h *servertest.Harness, method, url string, headers map[string]string) int {
	t.Helper()
	base, _, _ := strings.Cut(url[len("http://"):], "/")
	req, err := http.NewRequest(method, url, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(login(t, h.StateDir, "http://"+base))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestWeb_WriteRequestOriginAndJSONCheck(t *testing.T) {
	h, base := startWeb(t)
	host := strings.TrimPrefix(base, "http://")
	hostname, port, _ := net.SplitHostPort(host)
	otherPort := "http://" + hostname + ":1"
	if port == "1" {
		otherPort = "http://" + hostname + ":2"
	}
	js := "application/json"
	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"no origin", map[string]string{"Content-Type": js}, 403},
		{"other host", map[string]string{"Origin": "http://evil.example", "Content-Type": js}, 403},
		{"other port", map[string]string{"Origin": otherPort, "Content-Type": js}, 403},
		{"null origin", map[string]string{"Origin": "null", "Content-Type": js}, 403},
		{"text/plain", map[string]string{"Origin": base, "Content-Type": "text/plain"}, 403},
		{"no content type", map[string]string{"Origin": base}, 403},
		{"valid", map[string]string{"Origin": base, "Content-Type": js}, 404},
		{"scheme ignored", map[string]string{"Origin": "https://" + host, "Content-Type": js}, 404},
		{"charset param", map[string]string{"Origin": base, "Content-Type": js + "; charset=utf-8"}, 404},
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		for _, tc := range cases {
			for _, path := range []string{"/v1/snapshot", "/nope"} {
				if got := webSend(t, h, method, base+path, tc.headers); got != tc.want {
					t.Errorf("%s %s (%s) = %d, want %d", method, path, tc.name, got, tc.want)
				}
			}
		}
	}
}

func TestWeb_GetNeedsNoOrigin(t *testing.T) {
	h, base := startWeb(t)
	if code, _ := webDo(t, h, http.MethodGet, base+"/v1/snapshot"); code != http.StatusOK {
		t.Errorf("GET without Origin = %d, want 200", code)
	}
}

// The token check runs before the Origin check, so an unauthenticated write is
// 401 even with a bad Origin.
func TestWeb_WriteTokenCheckComesFirst(t *testing.T) {
	_, base := startWeb(t)
	if code, _ := webDoCookie(t, http.MethodPost, base+"/nope", nil); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated POST = %d, want 401", code)
	}
}

func TestWeb_OffByDefault(t *testing.T) {
	hs, err := servertest.Start(t).Client.Handshake(context.Background())
	if err != nil || hs.WebURL != "" || hs.WebError != "" {
		t.Errorf("handshake web fields = %q %q (err %v), want empty", hs.WebURL, hs.WebError, err)
	}
}

// The embed is the placeholder or a built dist, depending on whether make
// web-build ran, so the test only checks that an HTML page is served.
func TestWeb_ServesIndexAtRootAndFallback(t *testing.T) {
	h, base := startWeb(t)
	for _, path := range []string{"/", "/index.html", "/some/client/route"} {
		code, body := webDo(t, h, http.MethodGet, base+path)
		if code != http.StatusOK || !strings.Contains(strings.ToLower(body), "<!doctype html>") {
			t.Errorf("GET %s = %d %q", path, code, body)
		}
	}
}

func TestWeb_SnapshotIsServed(t *testing.T) {
	h, base := startWeb(t)
	if code, _ := webDo(t, h, http.MethodGet, base+"/v1/snapshot"); code != http.StatusOK {
		t.Errorf("GET /v1/snapshot = %d", code)
	}
}

func TestWeb_EverythingElseIs404(t *testing.T) {
	h, base := startWeb(t)
	validWrite := map[string]string{"Origin": base, "Content-Type": "application/json"}
	for _, pattern := range server.RoutePatterns() {
		method, path, _ := strings.Cut(pattern, " ")
		if pattern == "GET /v1/snapshot" || pattern == "GET /v1/events" || pattern == "GET /v1/tickets/detail" ||
			pattern == "GET /v1/epics/detail" ||
			pattern == "POST /v1/tickets/answer" || pattern == "POST /v1/tickets/unpark" || pattern == "POST /v1/agents/answer" {
			continue
		}
		if code := webSend(t, h, method, base+path, validWrite); code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", pattern, code)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		for _, path := range []string{"/v1/snapshot", "/v1/events", "/"} {
			if code := webSend(t, h, method, base+path, validWrite); code != http.StatusNotFound {
				t.Errorf("%s %s = %d, want 404", method, path, code)
			}
		}
	}
}

// Vite appends ?t=<timestamp> to modules it reloaded; that is not a login
// attempt, so a signed-in browser still gets the module.
func TestWeb_ViteTimestampParamIsNotATokenAttempt(t *testing.T) {
	h, base := startWeb(t)
	cookie := login(t, h.StateDir, base)
	if code, _ := webDoCookie(t, http.MethodGet, base+"/src/main.tsx?t=1792556497813", cookie); code == http.StatusUnauthorized {
		t.Errorf("status %d with a valid cookie, want the request let through", code)
	}
	if code, _ := webDoCookie(t, http.MethodGet, base+"/src/main.tsx?t=1792556497813", nil); code != http.StatusUnauthorized {
		t.Errorf("status %d without a cookie, want 401", code)
	}
	token, err := server.ReadWebToken(h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := webDoCookie(t, http.MethodGet, base+"/?t="+token, nil); code != http.StatusUnauthorized {
		t.Errorf("status %d for the old ?t= link, want 401", code)
	}
}

func TestWeb_ValidTokenSetsCookieAndRedirects(t *testing.T) {
	h, base := startWeb(t)
	token, err := server.ReadWebToken(h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := noRedirect.Get(base + "/some/route?gx_token=" + token)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Errorf("status %d, Location %q, want 303 to /", resp.StatusCode, resp.Header.Get("Location"))
	}
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.MaxAge != 365*24*60*60 {
		t.Errorf("cookie = %+v, want HttpOnly, SameSite=Strict and a one-year max age", c)
	}
	if strings.Contains(c.Value, token) {
		t.Errorf("cookie value contains the token itself")
	}
}

func TestWeb_BadOrMissingTokenIs401(t *testing.T) {
	_, base := startWeb(t)
	for _, path := range []string{"/", "/?gx_token=nope", "/v1/snapshot", "/v1/events", "/v1/snapshot?gx_token=nope"} {
		code, body := webDoCookie(t, http.MethodGet, base+path, nil)
		if code != http.StatusUnauthorized || body == "" {
			t.Errorf("GET %s = %d %q, want 401 with a page", path, code, body)
		}
	}
	code, _ := webDoCookie(t, http.MethodGet, base+"/", &http.Cookie{Name: "gx_web", Value: "forged"})
	if code != http.StatusUnauthorized {
		t.Errorf("forged cookie = %d, want 401", code)
	}
}

func TestWeb_CookieAuthenticatesEventStream(t *testing.T) {
	h, base := startWeb(t)
	req, err := http.NewRequest(http.MethodGet, base+"/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(login(t, h.StateDir, base))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /v1/events = %d, want 200", resp.StatusCode)
	}
}

func TestWeb_TokenChangeInvalidatesOldCookies(t *testing.T) {
	h, base := startWeb(t)
	old := login(t, h.StateDir, base)
	if err := os.WriteFile(server.WebTokenPath(h.StateDir), []byte("a-new-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.Restart(t)
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := webDoCookie(t, http.MethodGet, hs.WebURL+"/", old); code != http.StatusUnauthorized {
		t.Errorf("old cookie after token change = %d, want 401", code)
	}
	if code, _ := webDoCookie(t, http.MethodGet, hs.WebURL+"/", login(t, h.StateDir, hs.WebURL)); code != http.StatusOK {
		t.Errorf("new cookie = %d, want 200", code)
	}
}

func TestWeb_RotateInvalidatesCookiesOfRunningServer(t *testing.T) {
	h, base := startWeb(t)
	old := login(t, h.StateDir, base)
	before, _ := server.ReadWebToken(h.StateDir)
	token, err := server.RotateWebToken(h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if token == before || len(token) != 64 {
		t.Errorf("rotated token %q, was %q: want a new 32-byte hex token", token, before)
	}
	if code, _ := webDoCookie(t, http.MethodGet, base+"/", old); code != http.StatusUnauthorized {
		t.Errorf("old cookie after rotate = %d, want 401", code)
	}
	if code, _ := webDoCookie(t, http.MethodGet, base+"/?gx_token="+before, nil); code != http.StatusUnauthorized {
		t.Errorf("old token after rotate = %d, want 401", code)
	}
	if code, _ := webDoCookie(t, http.MethodGet, base+"/", login(t, h.StateDir, base)); code != http.StatusOK {
		t.Errorf("new cookie = %d, want 200", code)
	}
	info, err := os.Stat(server.WebTokenPath(h.StateDir))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("token file after rotate: %v, %v, want mode 0600", info, err)
	}
}

func TestWeb_TokenFileIs0600AndSurvivesRestart(t *testing.T) {
	h, _ := startWeb(t)
	info, err := os.Stat(server.WebTokenPath(h.StateDir))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("token file mode = %v, want 0600", info.Mode().Perm())
	}
	before, _ := server.ReadWebToken(h.StateDir)
	h.Restart(t)
	if after, _ := server.ReadWebToken(h.StateDir); after != before || len(before) != 64 {
		t.Errorf("token changed across restart or is not 32 hex-encoded bytes: %q -> %q", before, after)
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
