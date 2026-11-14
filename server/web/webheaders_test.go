package web_test

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

func headerGet(t *testing.T, url string, cookie *http.Cookie, host string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if host != "" {
		req.Host = host
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestWeb_SecurityHeaders_NonFileResponses(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	cookie := servertest.Login(t, h.StateDir, base)
	cases := []struct {
		name   string
		url    string
		cookie *http.Cookie
		host   string
		want   int
	}{
		{"app shell", base + "/", nil, "", http.StatusOK},
		{"snapshot 200", base + "/v1/snapshot", cookie, "", http.StatusOK},
		{"snapshot 401", base + "/v1/snapshot", nil, "", http.StatusUnauthorized},
		{"404 under v1", base + "/v1/nope", cookie, "", http.StatusNotFound},
		{"event stream", base + "/v1/events", cookie, "", http.StatusOK},
		{"refused host", base + "/", nil, "evil.example", http.StatusForbidden},
	}
	for _, tc := range cases {
		resp := headerGet(t, tc.url, tc.cookie, tc.host)
		if resp.StatusCode != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, resp.StatusCode, tc.want)
		}
		if got := resp.Header.Values("Content-Security-Policy"); len(got) != 1 || got[0] != "frame-ancestors 'none'" {
			t.Errorf("%s: CSP = %q", tc.name, got)
		}
		if got := resp.Header.Get("Referrer-Policy"); got != "same-origin" {
			t.Errorf("%s: Referrer-Policy = %q", tc.name, got)
		}
		if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: nosniff = %q", tc.name, got)
		}
	}
}

func TestWeb_SecurityHeaders_OriginRefusal(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	req, err := http.NewRequest(http.MethodPost, base+"/v1/snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(servertest.Login(t, h.StateDir, base))
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
	if got := resp.Header.Values("Content-Security-Policy"); len(got) != 1 || got[0] != "frame-ancestors 'none'" {
		t.Errorf("CSP = %q", got)
	}
	if resp.Header.Get("Referrer-Policy") != "same-origin" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers = %v", resp.Header)
	}
}

func TestWeb_SecurityHeaders_FileResponsesKeepTheirOwn(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	page := filepath.Join(f.repo, "site", "index.html")
	writeFile(t, page, "<p>site</p>")
	raw, _ := rawGet(t, f.base, "/v1/files"+page, f.cookie)
	token, _ := rawGet(t, f.base, raw.Header.Get("Location"), nil)
	denied, _ := rawGet(t, f.base, "/v1/files"+page, nil)
	for name, resp := range map[string]*http.Response{"raw 302": raw, "token 200": token, "raw 401": denied} {
		csp := resp.Header.Values("Content-Security-Policy")
		if len(csp) != 1 || !strings.HasPrefix(csp[0], "sandbox allow-scripts; frame-ancestors 'self'") {
			t.Errorf("%s: CSP = %q", name, csp)
		}
		if got := resp.Header.Get("Referrer-Policy"); got != "no-referrer" {
			t.Errorf("%s: Referrer-Policy = %q", name, got)
		}
		if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: nosniff = %q", name, got)
		}
	}
}
