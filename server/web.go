package server

import (
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"

	"github.com/elentok/gx/webui"
)

// bindWeb opens the web listener. A failure is recorded for the handshake and
// never stops the server. New calls it once, before anything can read s.web or
// s.webErr, so the writes need no lock.
func (s *Server) bindWeb() {
	if s.cfg.WebAddr == "" {
		return
	}
	if _, err := loadOrCreateWebToken(s.cfg.StateDir); err != nil {
		s.webErr = "web token: " + err.Error()
		s.log.Error("web token unavailable", "err", err)
		return
	}
	if s.cfg.WebDevURL != "" {
		proxy, err := devFrontendProxy(s.cfg.WebDevURL)
		if err != nil {
			s.webErr = err.Error()
			s.log.Error("web dev frontend rejected", "err", err)
			return
		}
		s.webDev = proxy
		s.log.Warn("web UI is served from a dev frontend", "url", s.cfg.WebDevURL)
	}
	ln, err := net.Listen("tcp", s.cfg.WebAddr)
	if err != nil {
		s.webErr = err.Error()
		s.log.Error("web listener failed to bind", "addr", s.cfg.WebAddr, "err", err)
		return
	}
	// An empty host (":8080") binds every interface but parses to a nil IP.
	if host, _, _ := net.SplitHostPort(s.cfg.WebAddr); host == "" || net.ParseIP(host).IsUnspecified() {
		s.log.Warn("web listener is bound to all interfaces and serves its token over plain HTTP", "addr", ln.Addr().String())
	}
	s.web = ln
	s.webHTTP = &http.Server{Handler: requireHost(func() []string { return s.settings.Load().WebAllowedHosts }, requireToken(s.cfg.StateDir, s.webHandler()))}
}

// WebURL is the web listener's base URL, or "" when it is off or failed to bind.
func (s *Server) WebURL() string {
	if s.web == nil {
		return ""
	}
	return "http://" + s.web.Addr().String()
}

// webHandler is the read-only mux: the snapshot, the event stream and the
// embedded assets. Every other request is a 404.
func (s *Server) webHandler() http.Handler {
	assets := webui.Assets()
	static := serveAsset(http.FileServerFS(assets), assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		switch {
		case r.URL.Path == "/v1/snapshot":
			s.snapshot(w, r)
		case r.URL.Path == "/v1/events":
			s.streamEvents(w, r)
		case strings.HasPrefix(r.URL.Path, "/v1/"):
			http.NotFound(w, r)
		case s.webDev != nil:
			s.webDev.ServeHTTP(w, r)
		default:
			static.ServeHTTP(w, r)
		}
	})
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
	return proxy, nil
}

// serveAsset serves the file at the request path. A path with no file gets
// index.html, so the single-page app can own its client-side routes.
func serveAsset(files http.Handler, assets fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if info, err := fs.Stat(assets, name); name != "" && (err != nil || info.IsDir()) {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			r = r2
		}
		files.ServeHTTP(w, r)
	})
}
