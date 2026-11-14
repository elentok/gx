package web_test

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/web"
)

func TestWebFiles_HeadersOn200_401_404(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	plan := "/v1/files" + f.repo + "/docs/plan.md"
	host := webHostPort(f.base)
	wantCSP := "sandbox allow-scripts; frame-ancestors 'self'; connect-src https://" + host
	for name, tc := range map[string]struct {
		target string
		cookie *http.Cookie
		status int
	}{
		"200": {plan, f.cookie, 200},
		"401": {plan, nil, 401},
		"404": {plan + "x", f.cookie, 404},
	} {
		resp, _ := rawGet(t, f.base, tc.target, tc.cookie)
		if resp.StatusCode != tc.status {
			t.Errorf("%s: status %d", name, resp.StatusCode)
		}
		want := map[string]string{
			"Content-Security-Policy": wantCSP,
			"X-Content-Type-Options":  "nosniff",
			"Referrer-Policy":         "no-referrer",
			"Cache-Control":           "private, no-store",
		}
		for k, v := range want {
			if got := resp.Header.Get(k); got != v {
				t.Errorf("%s: %s = %q, want %q", name, k, got, v)
			}
		}
		for _, k := range []string{"Cross-Origin-Resource-Policy", "Access-Control-Allow-Origin"} {
			if got := resp.Header.Get(k); got != "" {
				t.Errorf("%s: %s = %q, want none", name, k, got)
			}
		}
	}
}

func TestWebFiles_ContentTypeTable(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	dir := filepath.Join(f.repo, "types")
	files := map[string]struct{ body, want string }{
		"a.html":  {"<p>", "text/html; charset=utf-8"},
		"a.css":   {"a{}", "text/css; charset=utf-8"},
		"a.js":    {"1", "text/javascript; charset=utf-8"},
		"a.json":  {"{}", "application/json"},
		"a.svg":   {"<svg/>", "image/svg+xml"},
		"a.png":   {"\x89PNG", "image/png"},
		"a.pdf":   {"%PDF", "application/pdf"},
		"a.md":    {"# x", "text/plain; charset=utf-8"},
		"a.weird": {"just text", "text/plain; charset=utf-8"},
		"a.bin":   {"\x00\x01\x02", "application/octet-stream"},
	}
	for name, c := range files {
		writeFile(t, filepath.Join(dir, name), c.body)
		resp, body := rawGet(t, f.base, "/v1/files"+dir+"/"+name, f.cookie)
		if resp.StatusCode != 200 || body != c.body {
			t.Errorf("%s: status %d body %q", name, resp.StatusCode, body)
		}
		if got := resp.Header.Get("Content-Type"); got != c.want {
			t.Errorf("%s: Content-Type %q, want %q", name, got, c.want)
		}
	}
}

func TestWebFiles_CSPConnectSrc(t *testing.T) {
	t.Parallel()
	allowed := []string{"gx.example"}
	for host, want := range map[string]string{
		"127.0.0.1:8443":    "connect-src https://127.0.0.1:8443",
		"GX.example.:8443":  "connect-src https://gx.example:8443",
		"[::1]:8443":        "connect-src https://[::1]:8443",
		"gx.example":        "connect-src https://gx.example",
		"gx.example:80;x":   "connect-src https://gx.example",
		"evil.example:8443": "connect-src 'none'",
		"":                  "connect-src 'none'",
	} {
		if got := web.FileCSP(host, allowed); !strings.HasSuffix(got, want) {
			t.Errorf("Host %q: %q, want suffix %q", host, got, want)
		}
	}
}
