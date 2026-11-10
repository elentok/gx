package web_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// startWeb starts a server with a web listener serving the built files. It
// returns the web base URL and the harness.
func startWeb(t *testing.T) (string, *servertest.Harness) {
	t.Helper()
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
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

// startWebDev starts a server and swaps its web UI to the Vite stub at dev.
func startWebDev(t *testing.T, dev *httptest.Server) (string, *servertest.Harness) {
	t.Helper()
	base, h := startWeb(t)
	h.Server.WebUseViteProxy(portOf(t, dev.URL))
	return base, h
}

func portOf(t *testing.T, raw string) int {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func echoPathServer(t *testing.T) *httptest.Server {
	t.Helper()
	dev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("dev:" + r.URL.Path))
	}))
	t.Cleanup(dev.Close)
	return dev
}

func TestWebDevProxy_SwapsTheUIWithoutRestartingTheListener(t *testing.T) {
	t.Parallel()
	dev := echoPathServer(t)
	base, h := startWeb(t)
	cookie := servertest.Login(t, h.StateDir, base)

	code, built := servertest.WebDoCookie(t, http.MethodGet, base+"/", cookie)
	if code != http.StatusOK || strings.HasPrefix(built, "dev:") {
		t.Fatalf("built page = %d %q, want the built index", code, built)
	}

	h.Server.WebUseViteProxy(portOf(t, dev.URL))
	if code, body := servertest.WebDoCookie(t, http.MethodGet, base+"/src/main.tsx", cookie); code != http.StatusOK || body != "dev:/src/main.tsx" {
		t.Errorf("proxied asset = %d %q, want the dev frontend's response", code, body)
	}

	h.Server.WebUseBuiltUI()
	if code, body := servertest.WebDoCookie(t, http.MethodGet, base+"/", cookie); code != http.StatusOK || body != built {
		t.Errorf("page after swapping back = %d %q, want the built index", code, body)
	}
	if got := h.Server.WebURL(); got != base {
		t.Errorf("web URL = %q, want %q: the listener must not restart", got, base)
	}
}

func TestWebDevProxy_ModulesNeedNoCookieButTheAPIDoes(t *testing.T) {
	t.Parallel()
	base, _ := startWebDev(t, echoPathServer(t))
	code, body := servertest.WebDoCookie(t, http.MethodGet, base+"/src/main.tsx?t=1792556497813", nil)
	if code != http.StatusOK || body != "dev:/src/main.tsx" {
		t.Errorf("module without a cookie = %d %q, want the dev frontend's response", code, body)
	}
	if code, _ := servertest.WebDoCookie(t, http.MethodGet, base+"/v1/snapshot", nil); code != http.StatusUnauthorized {
		t.Errorf("API without a cookie = %d, want 401", code)
	}
}

func TestWebDevProxy_SnapshotStaysLocal(t *testing.T) {
	t.Parallel()
	dev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("dev frontend got %s", r.URL.Path)
	}))
	t.Cleanup(dev.Close)

	base, h := startWebDev(t, dev)
	cookie := servertest.Login(t, h.StateDir, base)
	if code, _ := servertest.WebDoCookie(t, http.MethodGet, base+"/v1/snapshot", cookie); code != http.StatusOK {
		t.Errorf("snapshot status = %d, want 200", code)
	}
}

func TestWebDevProxy_ExplainsWhenDevFrontendIsDown(t *testing.T) {
	t.Parallel()
	dev := httptest.NewServer(http.NotFoundHandler())
	port := portOf(t, dev.URL)
	dev.Close()

	base, h := startWeb(t)
	h.Server.WebUseViteProxy(port)
	cookie := servertest.Login(t, h.StateDir, base)
	code, body := servertest.WebDoCookie(t, http.MethodGet, base+"/", cookie)
	if code != http.StatusBadGateway || !strings.Contains(body, strconv.Itoa(port)) || strings.Contains(body, "npm run dev") {
		t.Errorf("page = %d %q, want 502 naming port %d without telling the person to run npm", code, body, port)
	}
}

func TestWebDevProxy_PassesWebsocketUpgrades(t *testing.T) {
	t.Parallel()
	dev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "not an upgrade", http.StatusBadRequest)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\nhmr-ping")
	}))
	t.Cleanup(dev.Close)

	base, h := startWebDev(t, dev)
	cookie := servertest.Login(t, h.StateDir, base)
	req, err := http.NewRequest(http.MethodGet, base+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.AddCookie(cookie)
	resp, err := servertest.WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want 101", resp.StatusCode)
	}
	buf := make([]byte, len("hmr-ping"))
	if _, err := io.ReadFull(resp.Body, buf); err != nil || string(buf) != "hmr-ping" {
		t.Errorf("upgraded stream = %q (err %v), want hmr-ping", buf, err)
	}
}
