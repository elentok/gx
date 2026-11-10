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
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/webui"
)

// checkWriteRequest returns why a non-GET request is refused, or "". It is the
// CSRF guard: a cross-site form post cannot set a JSON content type, and a
// cross-site fetch cannot forge a same-host Origin. Origin and Host are
// compared on host and port; the scheme is ignored. A rawBody route (see
// wire.Route.RawBody) skips the content-type rule: a cross-site fetch still
// cannot forge the Origin.
func checkWriteRequest(r *http.Request, rawBody bool) string {
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		return "missing or opaque Origin"
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || !strings.EqualFold(u.Host, r.Host) {
		return "Origin does not match Host"
	}
	if rawBody {
		return ""
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		return "Content-Type must be application/json"
	}
	return ""
}

// devFrontendProxy forwards requests to the Vite dev server gx started on a
// loopback port. The Host header is rewritten so Vite accepts it, and
// websocket upgrades pass through for hot reload. The target is always
// 127.0.0.1, so the listener cannot be pointed at other hosts.
func devFrontendProxy(port int) http.Handler {
	u := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}
	proxy := httputil.NewSingleHostReverseProxy(u)
	direct := proxy.Director
	proxy.Director = func(r *http.Request) {
		direct(r)
		r.Host = u.Host
	}
	// Without this the browser gets an empty 502 and shows a blank page.
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, fmt.Sprintf("502: the Vite dev server that gx started at %s is not answering (%v).", u.Host, err), http.StatusBadGateway)
	}
	// The built worker would cache the shell and hide Vite's pages; this one
	// removes any such worker from the browser instead of forwarding to Vite.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sw.js" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(webui.DevWorker)
			return
		}
		proxy.ServeHTTP(w, r)
	})
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
		// Go's mime table has no .webmanifest entry, and .json would get application/json.
		if name == "manifest.webmanifest" || name == "manifest.json" {
			w.Header().Set("Content-Type", "application/manifest+json")
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
