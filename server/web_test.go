package server_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// noRedirect lets a test see the redirect itself instead of its target.
var noRedirect = &http.Client{
	Transport:     servertest.WebClient().Transport,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// webHostPort returns the host:port of a web URL.
func webHostPort(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return u.Host
}

// webSend sends an authenticated request with the given headers and an empty JSON body.
func webSend(t *testing.T, h *servertest.Harness, method, url string, headers map[string]string) int {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(servertest.Login(t, h.StateDir, servertest.WebBase(url)))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := servertest.WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestWeb_WriteRequestOriginAndJSONCheck(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	host := webHostPort(base)
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
	t.Parallel()
	h, base := servertest.StartWeb(t)
	if code, _ := servertest.WebDo(t, h, http.MethodGet, base+"/v1/snapshot"); code != http.StatusOK {
		t.Errorf("GET without Origin = %d, want 200", code)
	}
}

// The login check runs before the Origin check, so an unauthenticated write is
// 401 even with a bad Origin.
func TestWeb_WriteLoginCheckComesFirst(t *testing.T) {
	t.Parallel()
	_, base := servertest.StartWeb(t)
	if code, _ := servertest.WebDoCookie(t, http.MethodPost, base+"/nope", nil); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated POST = %d, want 401", code)
	}
}

func TestWeb_OffByDefault(t *testing.T) {
	t.Parallel()
	hs, err := servertest.Start(t).Client.Handshake(context.Background())
	if err != nil || hs.WebURL != "" || hs.WebError != "" {
		t.Errorf("handshake web fields = %q %q (err %v), want empty", hs.WebURL, hs.WebError, err)
	}
}

// webGet sends an authenticated GET with extra headers and returns the response.
func webGet(t *testing.T, h *servertest.Harness, url string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(servertest.Login(t, h.StateDir, servertest.WebBase(url)))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := servertest.WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// The embed is the placeholder or a built dist, depending on whether make
// web-build ran, so this checks the root and the 404s; the asset rules run
// against a fake build in TestServeAsset_CacheHeaders.
func TestWeb_ServesIndexOnlyAtRoot(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	resp := webGet(t, h, base+"/", nil)
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") ||
		!strings.Contains(strings.ToLower(string(body)), "<!doctype html>") {
		t.Fatalf("GET / = %d %s %q", resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("GET / Cache-Control = %q, want no-cache", got)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("GET / has no ETag")
	}
	if got := webGet(t, h, base+"/", map[string]string{"If-None-Match": etag}).StatusCode; got != http.StatusNotModified {
		t.Errorf("GET / with matching If-None-Match = %d, want 304", got)
	}
	for _, path := range []string{"/assets/missing.js", "/stats", "/some/client/route", "/index.html"} {
		if code, _ := servertest.WebDo(t, h, http.MethodGet, base+path); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, code)
		}
	}
}

func TestServeAsset_CacheHeaders(t *testing.T) {
	t.Parallel()
	handler := server.ServeAsset(fstest.MapFS{
		"index.html":              {Data: []byte("<!doctype html>")},
		"apple-touch-icon.png":    {Data: []byte("png")},
		"assets/index-AbC123.js":  {Data: []byte("js")},
		"assets/index-AbC123.css": {Data: []byte("css")},
	})
	cases := []struct{ path, cache string }{
		{"/", "no-cache"},
		{"/apple-touch-icon.png", "no-cache"},
		{"/assets/index-AbC123.js", "max-age=31536000, immutable"},
		{"/assets/index-AbC123.css", "max-age=31536000, immutable"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != tc.cache {
			t.Errorf("GET %s = %d Cache-Control %q, want 200 %q", tc.path, rec.Code, rec.Header().Get("Cache-Control"), tc.cache)
		}
	}
	for _, path := range []string{"/assets/missing.js", "/assets/", "/assets", "/stats"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}

func TestWeb_SnapshotIsServed(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	if code, _ := servertest.WebDo(t, h, http.MethodGet, base+"/v1/snapshot"); code != http.StatusOK {
		t.Errorf("GET /v1/snapshot = %d", code)
	}
}

func TestWeb_SessionReadsAreServed(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	if code, _ := servertest.WebDo(t, h, http.MethodGet, base+"/v1/sessions"); code != http.StatusOK {
		t.Errorf("GET /v1/sessions = %d", code)
	}
	// A bad id is the handler's 400; an unserved route would be a 404.
	if code, _ := servertest.WebDo(t, h, http.MethodGet, base+"/v1/sessions/chat?session=../x"); code != http.StatusBadRequest {
		t.Errorf("GET /v1/sessions/chat = %d, want 400", code)
	}
	if code, _ := servertest.WebDo(t, h, http.MethodGet, base+"/v1/sessions/screen?session=../x"); code != http.StatusBadRequest {
		t.Errorf("GET /v1/sessions/screen = %d, want 400", code)
	}
}

func TestWeb_HealthReadIsServed(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	if code, _ := servertest.WebDo(t, h, http.MethodGet, base+"/v1/health"); code != http.StatusOK {
		t.Errorf("GET /v1/health = %d", code)
	}
}

func TestUsageSummary_WebServesRead(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	if code, _ := servertest.WebDo(t, h, http.MethodGet, base+"/v1/usage/summary"); code != http.StatusOK {
		t.Errorf("GET /v1/usage/summary = %d", code)
	}
}

func TestWeb_EverythingElseIs404(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	validWrite := map[string]string{"Origin": base, "Content-Type": "application/json"}
	for _, pattern := range server.RoutePatterns() {
		method, path, _ := strings.Cut(pattern, " ")
		if pattern == "GET /v1/snapshot" || pattern == "GET /v1/events" || pattern == "GET /v1/tickets/detail" ||
			pattern == "GET /v1/epics/detail" || pattern == "GET /v1/epics" || pattern == "GET /v1/stats" || pattern == "GET /v1/health" || pattern == "GET /v1/usage/summary" || pattern == "GET /v1/restart/check" || pattern == "GET /v1/projects/worktrees" ||
			pattern == "GET /v1/checks/status" || pattern == "GET /v1/settings/agents" || pattern == "POST /v1/settings/agents" || pattern == "GET /v1/sessions" || pattern == "GET /v1/sessions/chat" || pattern == "GET /v1/sessions/chat/archive" || pattern == "GET /v1/sessions/screen" || pattern == "GET /v1/sessions/commands" || pattern == "GET /v1/sessions/permission" ||
			pattern == "POST /v1/tickets/answer" || pattern == "POST /v1/tickets/unpark" || pattern == "POST /v1/tickets/resume" || pattern == "POST /v1/agents/answer" ||
			pattern == "POST /v1/tickets/retry" || pattern == "POST /v1/tickets/merge" || pattern == "POST /v1/tickets/start-over" ||
			pattern == "POST /v1/sessions/stop" || pattern == "POST /v1/sessions/remove" || pattern == "POST /v1/sessions/resume" || pattern == "POST /v1/designs/open" || pattern == "POST /v1/designs/delete" || pattern == "POST /v1/tickets/fix-agent" || pattern == "POST /v1/projects/retry-migration" || pattern == "POST /v1/sessions/prompt" || pattern == "POST /v1/sessions/keys" || pattern == "POST /v1/sessions/create" || pattern == "POST /v1/sessions/interrupt" || pattern == "POST /v1/sessions/auto-advance" || pattern == "POST /v1/sessions/auto-advance/cancel" || pattern == "POST /v1/sessions/focus" ||
			pattern == "POST /v1/queue/add" || pattern == "POST /v1/epics/merge" || pattern == "POST /v1/epics/stash-merge" || pattern == "POST /v1/afk" || pattern == "POST /v1/vimode" || pattern == "POST /v1/usage/override" || pattern == "POST /v1/checks/save" || pattern == "POST /v1/disk/clean" || pattern == "POST /v1/restart" {
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

// An old sign-in link or an old gx_web cookie is not a login: the page is served
// like for any visitor, nothing is set, and the API stays closed.
func TestWeb_OldTokenLinkAndCookieAreIgnored(t *testing.T) {
	t.Parallel()
	_, base := servertest.StartWeb(t)
	req, err := http.NewRequest(http.MethodGet, base+"/?gx_token=abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: "gx_web", Value: "x"})
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(strings.ToLower(string(body)), "<!doctype html>") {
		t.Errorf("GET /?gx_token = %d %q, want the HTML page", resp.StatusCode, body)
	}
	if len(resp.Cookies()) != 0 {
		t.Errorf("old token link set cookies %v, want none", resp.Cookies())
	}
	if code, _ := servertest.WebDoCookie(t, http.MethodGet, base+"/v1/snapshot?gx_token=abc", &http.Cookie{Name: "gx_web", Value: "x"}); code != http.StatusUnauthorized {
		t.Errorf("snapshot with an old token and cookie = %d, want 401", code)
	}
}

func TestWeb_StartDeletesTheOldTokenFile(t *testing.T) {
	t.Parallel()
	h, _ := servertest.StartWeb(t)
	path := filepath.Join(h.StateDir, "web-token")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.Restart(t)
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("web-token after start: %v, want it deleted", err)
	}
}

func TestWeb_CookieAuthenticatesEventStream(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	req, err := http.NewRequest(http.MethodGet, base+"/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(servertest.Login(t, h.StateDir, base))
	resp, err := servertest.WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /v1/events = %d, want 200", resp.StatusCode)
	}
}

func TestWeb_BindFailureIsReportedAndServerKeepsRunning(t *testing.T) {
	t.Parallel()
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

// Browsers share cookies across ports on one host, so two servers must not
// overwrite each other's session.
func TestWeb_TwoServersKeepSeparateCookies(t *testing.T) {
	t.Parallel()
	h1, base1 := servertest.StartWeb(t)
	h2, base2 := servertest.StartWeb(t)
	c1, c2 := servertest.Login(t, h1.StateDir, base1), servertest.Login(t, h2.StateDir, base2)
	if c1.Name == c2.Name {
		t.Fatalf("both servers use cookie %q", c1.Name)
	}
	for _, tc := range []struct {
		base    string
		cookies []*http.Cookie
	}{{base1, []*http.Cookie{c1, c2}}, {base2, []*http.Cookie{c2, c1}}} {
		req, _ := http.NewRequest(http.MethodGet, tc.base+"/v1/snapshot", nil)
		for _, c := range tc.cookies {
			req.AddCookie(c)
		}
		resp, err := servertest.WebClient().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s with both cookies = %d, want 200", tc.base, resp.StatusCode)
		}
	}
}
