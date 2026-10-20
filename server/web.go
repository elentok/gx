package server

import (
	"io/fs"
	"net"
	"net/http"
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
	s.webHTTP = &http.Server{Handler: requireToken(s.cfg.StateDir, s.webHandler())}
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
		default:
			static.ServeHTTP(w, r)
		}
	})
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
