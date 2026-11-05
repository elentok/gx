package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"
)

// checkWriteRequest returns why a non-GET request is refused, or "". It is the
// CSRF guard: a cross-site form post cannot set a JSON content type, and a
// cross-site fetch cannot forge a same-host Origin. Origin and Host are
// compared on host and port; the scheme is ignored.
func checkWriteRequest(r *http.Request) string {
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		return "missing or opaque Origin"
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || !strings.EqualFold(u.Host, r.Host) {
		return "Origin does not match Host"
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		return "Content-Type must be application/json"
	}
	return ""
}

// devFrontendProxy forwards requests to a Vite dev server on loopback. The
// Host header is rewritten so Vite accepts it, and websocket upgrades pass
// through for hot reload. Only loopback targets are allowed, so the read-only
// listener cannot be pointed at other hosts.
func devFrontendProxy(raw string) (http.Handler, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Host == "" {
		return nil, fmt.Errorf("server.web.dev-url %q must be an http URL", raw)
	}
	host := u.Hostname()
	if host != "localhost" && !(net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()) {
		return nil, fmt.Errorf("server.web.dev-url %q must be a loopback address", raw)
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	direct := proxy.Director
	proxy.Director = func(r *http.Request) {
		direct(r)
		r.Host = u.Host
	}
	// Without this the browser gets an empty 502 and shows a blank page.
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, fmt.Sprintf("502: the dev frontend at %s is not answering (%v). Start it with `npm run dev` in webui/, or turn off server.web.dev-frontend.", raw, err), http.StatusBadGateway)
	}
	return proxy, nil
}

// ServeAsset serves index.html at "/" only, and the file at any other path.
// A path with no file is a 404, so a stale page fails loudly instead of
// loading HTML as a script. The app uses a hash router, so it needs no fallback.
func ServeAsset(assets fs.FS) http.Handler {
	files := http.FileServerFS(assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			serveIndex(w, r, assets)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		info, err := fs.Stat(assets, name)
		if err != nil || info.IsDir() || name == "index.html" {
			http.NotFound(w, r)
			return
		}
		// Vite names files under assets/ by content hash; other root files keep their name.
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// serveIndex answers with index.html and an ETag from its content, so a
// matching If-None-Match gets 304.
func serveIndex(w http.ResponseWriter, r *http.Request, assets fs.FS) {
	body, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	sum := sha256.Sum256(body)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:])+`"`)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(body))
}
