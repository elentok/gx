package server_test

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// plainNoRedirect is a plain-HTTP client that shows the redirect itself.
var plainNoRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func plainGet(t *testing.T, url, host string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		req.Host = host
	}
	resp, err := plainNoRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestWebTLS_OnePortServesHTTPSAndRedirectsPlainHTTP(t *testing.T) {
	t.Parallel()
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.WebAllowedHosts = []string{"gx.example.org"}
	})
	hs, err := h.Client.Handshake(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hs.WebURL, "https://") {
		t.Fatalf("web_url = %q, want https", hs.WebURL)
	}
	// The advertised URL names the allowed host, which does not resolve here, so dial loopback.
	advertised := webHostPort(hs.WebURL)
	port := advertised[strings.LastIndex(advertised, ":"):]
	host := "127.0.0.1" + port
	webURL := "https://" + host

	if code, _ := webDo(t, h, http.MethodGet, webURL+"/v1/snapshot"); code != http.StatusOK {
		t.Errorf("https snapshot = %d, want 200", code)
	}

	cases := []struct {
		name, host, wantLocation string
	}{
		{"ip", "", "https://" + host + "/v1/stats"},
		{"allowed name", "gx.example.org" + port, "https://gx.example.org" + port + "/v1/stats"},
		{"localhost", "localhost" + port, "https://localhost" + port + "/v1/stats"},
	}
	for _, c := range cases {
		resp := plainGet(t, "http://"+host+"/v1/stats", c.host)
		if resp.StatusCode < 300 || resp.StatusCode > 399 {
			t.Errorf("%s: plain GET = %d, want 3xx", c.name, resp.StatusCode)
		}
		if got := resp.Header.Get("Location"); got != c.wantLocation {
			t.Errorf("%s: Location = %q, want %q", c.name, got, c.wantLocation)
		}
	}

	resp := plainGet(t, "http://"+host+"/v1/stats", "evil.example")
	if resp.StatusCode != http.StatusForbidden || resp.Header.Get("Location") != "" {
		t.Errorf("plain GET with unlisted Host = %d (Location %q), want 403 and no redirect", resp.StatusCode, resp.Header.Get("Location"))
	}

	// Each name works on its own over HTTPS, with no redirect between hosts.
	cookie := login(t, h.StateDir, webURL)
	for _, name := range []string{"gx.example.org" + port, "localhost" + port, host} {
		if code, _ := hostStatus(t, webURL, name, cookie); code != http.StatusOK {
			t.Errorf("https Host %q = %d, want 200", name, code)
		}
	}
}

func TestWebTLS_NoConfigKeyTurnsItOff(t *testing.T) {
	t.Parallel()
	schema, err := os.ReadFile("../docs/config-schema.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"tls"`, `"https"`, `"insecure"`, `"plain"`} {
		if strings.Contains(string(schema), key) {
			t.Errorf("config schema has %s; no key may turn web TLS off", key)
		}
	}
}
