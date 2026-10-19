package server

import (
	"errors"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"path"
	"strings"

	"github.com/elentok/gx/webui"
)

// bindWeb opens the web listener. A failure is recorded for the handshake and
// never stops the server.
func (s *Server) bindWeb() {
	if s.cfg.WebAddr == "" {
		return
	}
	ln, err := net.Listen("tcp", s.cfg.WebAddr)
	if err != nil {
		s.webErr = err.Error()
		s.log.Error("web listener failed to bind", "addr", s.cfg.WebAddr, "err", err)
		return
	}
	if host, _, _ := net.SplitHostPort(s.cfg.WebAddr); net.ParseIP(host).IsUnspecified() {
		s.log.Warn("web listener is bound to all interfaces and has no auth", "addr", ln.Addr().String())
	}
	s.web = ln
	s.webHTTP = &http.Server{Handler: s.webHandler()}
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
			serveAsset(w, r, assets)
		}
	})
}

// serveAsset serves the file at the request path. A path with no file gets
// index.html, so the single-page app can own its client-side routes.
func serveAsset(w http.ResponseWriter, r *http.Request, assets fs.FS) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "index.html" {
		name = "index.html"
	} else if info, err := fs.Stat(assets, name); err != nil || info.IsDir() {
		name = "index.html"
	}
	data, err := fs.ReadFile(assets, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(data)
}
