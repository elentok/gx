package server_test

import (
	"bufio"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// hostStatus sends GET /v1/snapshot to base with the given Host header and
// returns the status and body.
func hostStatus(t *testing.T, base, host string, cookie *http.Cookie) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+"/v1/snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := webClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	_, _ = bufio.NewReader(resp.Body).WriteTo(&sb)
	return resp.StatusCode, sb.String()
}

func startWebHosts(t *testing.T, hosts ...string) (*servertest.Harness, string, string) {
	t.Helper()
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.WebAllowedHosts = hosts
	})
	hs, err := h.Client.Handshake(t.Context())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	_, port, _ := net.SplitHostPort(webHostPort(hs.WebURL))
	return h, hs.WebURL, port
}

func TestWebHost_Table(t *testing.T) {
	t.Parallel()
	h, base, port := startWebHosts(t, "gx.tail1234.ts.net")
	cookie := login(t, h.StateDir, base)
	tests := []struct {
		name string
		host string
		want int
	}{
		{"ipv4 loopback", "127.0.0.1:" + port, http.StatusOK},
		{"ipv6 loopback", "[::1]:" + port, http.StatusOK},
		{"ipv6 without port", "[::1]", http.StatusOK},
		{"localhost", "localhost:" + port, http.StatusOK},
		{"localhost uppercase", "LOCALHOST:" + port, http.StatusOK},
		{"localhost trailing dot", "localhost.:" + port, http.StatusOK},
		{"lan ip", "192.168.1.20:" + port, http.StatusOK},
		{"tailscale ip", "100.101.102.103:" + port, http.StatusOK},
		{"unlisted name", "evil.example:" + port, http.StatusForbidden},
		{"unlisted name without port", "evil.example", http.StatusForbidden},
		{"listed magicdns name", "gx.tail1234.ts.net:" + port, http.StatusOK},
		{"listed name other case", "GX.Tail1234.TS.net:" + port, http.StatusOK},
		{"listed name trailing dot", "gx.tail1234.ts.net.:" + port, http.StatusOK},
		{"listed name with any port", "gx.tail1234.ts.net:9", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code, body := hostStatus(t, base, tt.host, cookie); code != tt.want {
				t.Errorf("Host %q = %d %q, want %d", tt.host, code, body, tt.want)
			}
		})
	}
}

func TestWebHost_AllRoutesRefuseUnlistedName(t *testing.T) {
	t.Parallel()
	h, base, _ := startWebHosts(t)
	cookie := login(t, h.StateDir, base)
	for _, path := range []string{"/", "/index.html", "/some/client/route", "/v1/snapshot", "/v1/events", "/v1/nope"} {
		req, _ := http.NewRequest(http.MethodGet, base+path, nil)
		req.Host = "evil.example"
		req.AddCookie(cookie)
		resp, err := webClient().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s = %d, want 403", path, resp.StatusCode)
		}
	}
}

func TestWebHost_RefusalIsShortPlainText(t *testing.T) {
	t.Parallel()
	_, base, _ := startWebHosts(t)
	req, _ := http.NewRequest(http.MethodGet, base+"/", nil)
	req.Host = "evil.example"
	resp, err := webClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	_, _ = bufio.NewReader(resp.Body).WriteTo(&sb)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	if body := sb.String(); !strings.HasPrefix(body, "403:") || len(body) > 200 {
		t.Errorf("body = %q, want a short 403 reason", body)
	}
}

func TestWebHost_WrongHostWithoutCookieIs403Not401(t *testing.T) {
	t.Parallel()
	_, base, port := startWebHosts(t, "gx.tail1234.ts.net")
	if code, _ := hostStatus(t, base, "evil.example:"+port, nil); code != http.StatusForbidden {
		t.Errorf("unlisted host, no cookie = %d, want 403", code)
	}
	if code, _ := hostStatus(t, base, "gx.tail1234.ts.net:"+port, nil); code != http.StatusUnauthorized {
		t.Errorf("listed host, no cookie = %d, want 401", code)
	}
}

func TestWebHost_MissingHostIs403(t *testing.T) {
	t.Parallel()
	_, base, _ := startWebHosts(t)
	conn, err := net.Dial("tcp", webHostPort(base))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("GET /v1/snapshot HTTP/1.0\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("HTTP/1.0 without Host = %d, want 403", resp.StatusCode)
	}
}

func TestWebHost_AnyHostIPLiteralPassesOnAllInterfacesBind(t *testing.T) {
	t.Parallel()
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) { c.WebAddr = "0.0.0.0:0" })
	hs, err := h.Client.Handshake(t.Context())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	_, port, _ := net.SplitHostPort(webHostPort(hs.WebURL))
	base := "https://127.0.0.1:" + port
	cookie := login(t, h.StateDir, base)
	for _, host := range []string{"0.0.0.0:" + port, "192.168.1.20:" + port, "100.64.0.1:" + port} {
		if code, _ := hostStatus(t, base, host, cookie); code != http.StatusOK {
			t.Errorf("Host %q = %d, want 200", host, code)
		}
	}
	if code, _ := hostStatus(t, base, "evil.example", cookie); code != http.StatusForbidden {
		t.Errorf("unlisted name on 0.0.0.0 bind = %d, want 403", code)
	}
}

// Seam 2 + reload: the list is swapped in place, so the same listener answers.
func TestWebHost_ReloadAppliesAllowedHostsWithoutRestart(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
	})
	base := h.Server.WebURL()
	cookie := login(t, h.StateDir, base)
	if code, _ := hostStatus(t, base, "new.example", cookie); code != http.StatusForbidden {
		t.Fatalf("before reload = %d, want 403", code)
	}

	servertest.WriteConfig(t, path, store, map[string]any{
		"server": map[string]any{"web": map[string]any{"allowed-hosts": []string{"new.example"}}},
	})
	res := reload(t, h)
	// The harness binds port 0, so the config's listen differs from WebAddr and
	// the reload warns about it; that warning is about listen, not allowed-hosts.
	if res.Refused || !slices.Contains(res.Changed, "server.web.allowed-hosts") || slices.Contains(res.Warnings, "restart needed for server.web.allowed-hosts") {
		t.Fatalf("reload = %+v, want allowed-hosts applied without a restart warning", res)
	}
	if h.Server.WebURL() != base {
		t.Fatalf("listener moved: %q -> %q", base, h.Server.WebURL())
	}
	if code, _ := hostStatus(t, base, "new.example", cookie); code != http.StatusOK {
		t.Errorf("after adding = %d, want 200", code)
	}

	servertest.WriteConfig(t, path, store, nil)
	reload(t, h)
	if code, _ := hostStatus(t, base, "new.example", cookie); code != http.StatusForbidden {
		t.Errorf("after removing = %d, want 403", code)
	}
}
