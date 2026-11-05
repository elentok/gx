// Package web is the web listener: it binds, serves the web mux behind the
// login gate and answers the CLI's web calls. The root server supplies what it
// cannot own through Host.
package web

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/webtls"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/webui"
)

// Config is what the web listener reads from the server config.
type Config struct {
	Addr         string // "" turns the web listener off
	DevURL       string
	StateDir     string
	Clock        func() time.Time // nil means time.Now
	InterfaceIPs func() []net.IP  // nil means the machine's interfaces
}

// Host is what the web mux asks the root server for.
type Host interface {
	Settings() *settings.Settings
	Routes() []wire.Route
	Snapshot(http.ResponseWriter, *http.Request)
	RestartCheckRead(http.ResponseWriter, *http.Request)
	RestartWrite(http.ResponseWriter, *http.Request)
	StreamEvents(http.ResponseWriter, *http.Request)
}

// Web owns the web listener and its login gate. After Bind, nothing writes its
// fields, so readers need no lock.
type Web struct {
	cfg  Config
	log  *slog.Logger
	host Host

	ln       net.Listener // nil unless Config.Addr is set and bound
	dev      http.Handler // nil unless Config.DevURL is set and valid
	srv      *http.Server
	err      string        // why the bind failed
	listenIP net.IP        // nil when the listener binds every interface
	certs    *webtls.Certs // nil unless the listener is bound
	auth     *webauth.Gate // nil unless the listener is bound
	streams  streams
	errs     ipLimiter
}

// New makes a Web that is off until Bind opens the listener.
func New(cfg Config, log *slog.Logger, host Host) *Web {
	return &Web{cfg: cfg, log: log, host: host}
}

// Listener is the bound listener, nil when the web UI is off or failed to bind.
func (w *Web) Listener() net.Listener { return w.ln }

// Err is why the bind failed, or "".
func (w *Web) Err() string { return w.err }

// Gate is the web login gate, nil unless the listener is bound.
func (w *Web) Gate() *webauth.Gate { return w.auth }

// Certs is the web certificate store, nil unless the listener is bound.
func (w *Web) Certs() *webtls.Certs { return w.certs }

// Serve blocks serving the bound listener.
func (w *Web) Serve() error { return w.srv.Serve(w.ln) }

// Shutdown stops the web server.
func (w *Web) Shutdown(ctx context.Context) error { return w.srv.Shutdown(ctx) }

// Track returns a context that ends with ctx or when the login's streams are
// closed, and the function that stops tracking it.
func (w *Web) Track(ctx context.Context, loginID string) (context.Context, func()) {
	return w.streams.track(ctx, loginID)
}

// Bind opens the web listener. A failure is recorded for the handshake and
// never stops the server. The server calls it once, before anything can read
// the Web, so the writes need no lock.
func (w *Web) Bind() {
	if w.cfg.Addr == "" {
		return
	}
	if err := removeLegacyWebToken(w.cfg.StateDir); err != nil {
		w.log.Warn("could not delete the old web token file", "err", err)
	}
	if w.cfg.DevURL != "" {
		proxy, err := devFrontendProxy(w.cfg.DevURL)
		if err != nil {
			w.err = err.Error()
			w.log.Error("web dev frontend rejected", "err", err)
			return
		}
		w.dev = proxy
		w.log.Warn("web UI is served from a dev frontend", "url", w.cfg.DevURL)
	}
	auth, err := webauth.OpenWebAuth(w.cfg.StateDir, w.now)
	if err != nil {
		w.err = "web auth: " + err.Error()
		w.log.Error("web auth unavailable", "err", err)
		return
	}
	rawLn, err := net.Listen("tcp", w.cfg.Addr)
	if err != nil {
		w.err = err.Error()
		w.log.Error("web listener failed to bind", "addr", w.cfg.Addr, "err", err)
		return
	}
	var listenIP net.IP
	if a, ok := rawLn.Addr().(*net.TCPAddr); ok && !a.IP.IsUnspecified() {
		listenIP = a.IP
	}
	allowed := func() []string { return w.host.Settings().WebAllowedHosts }
	certs, err := webtls.NewCerts(&webtls.Certs{
		StateDir: w.cfg.StateDir,
		ListenIP: listenIP,
		Now:      w.now,
		Hosts:    allowed,
		IfaceIPs: w.interfaceIPs,
		Log:      w.log,
	})
	if err != nil {
		rawLn.Close()
		w.err = "web tls: " + err.Error()
		w.log.Error("web tls unavailable", "err", err)
		return
	}
	ln := webtls.NewSniffListener(webtls.NewSourceIPListener(rawLn, w.log, time.Now), &tls.Config{GetCertificate: certs.GetCertificate, MinVersion: tls.VersionTLS12})
	w.ln, w.listenIP, w.certs = ln, listenIP, certs
	w.auth = webauth.NewGate(webauth.Config{
		Store:    auth,
		Cookie:   webauth.LoginCookieName(ln),
		TOTPOn:   func() bool { return !w.host.Settings().WebTOTPOff },
		RootCert: func() *x509.Certificate { return w.certs.Root() },
		Log:      w.log,
		Now:      w.now,
		Host:     authHost{w},
	})
	mux := w.handler()
	protected := w.auth.RequireLogin(mux, w.auth.Deny(mux))
	w.srv = &http.Server{Handler: webtls.RequireHost(allowed, webtls.RedirectPlainHTTP(w.servePublicFirst(mux, protected)))}
}

// URL is the web listener's base URL, or "" when it is off or failed to bind.
func (w *Web) URL() string {
	if w.ln == nil {
		return ""
	}
	_, port, _ := net.SplitHostPort(w.ln.Addr().String())
	return webtls.BaseURL(w.host.Settings().WebAllowedHosts, w.certs.Root(), w.listenIP, port, w.interfaceIPs())
}

// interfaceIPs is the one place the machine's interfaces are read.
func (w *Web) interfaceIPs() []net.IP {
	if w.cfg.InterfaceIPs != nil {
		return w.cfg.InterfaceIPs()
	}
	return webtls.InterfaceIPs()
}

func (w *Web) now() time.Time {
	if w.cfg.Clock != nil {
		return w.cfg.Clock()
	}
	return time.Now()
}

// handler is the web mux: the snapshot, the event stream and the embedded
// assets, plus the allowlisted writes. Every other request is a 404.
func (w *Web) handler() http.Handler {
	static := ServeAsset(webui.Assets())
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		routes := w.host.Routes()
		if !w.isRead(r) {
			if reason := checkWriteRequest(r); reason != "" {
				http.Error(rw, reason, http.StatusForbidden)
				return
			}
			key := r.Method + " " + r.URL.Path
			if handler, ok := webWrites[key]; ok {
				handler(w)(rw, r)
				return
			}
			if route, ok := FindRoute(routes, key, wire.WebWrite, wire.WebPublic); ok {
				serveRoute(route)(rw, r)
				return
			}
			http.NotFound(rw, r)
			return
		}
		if route, ok := FindRoute(routes, "GET "+r.URL.Path, wire.WebPublic, wire.WebRead); ok {
			serveRoute(route)(rw, r)
			return
		}
		switch {
		case r.URL.Path == "/v1/snapshot":
			w.host.Snapshot(rw, r)
		case r.URL.Path == "/v1/restart/check":
			w.host.RestartCheckRead(rw, r)
		case r.URL.Path == "/v1/events":
			w.host.StreamEvents(rw, r)
		case strings.HasPrefix(r.URL.Path, "/v1/"):
			http.NotFound(rw, r)
		case w.dev != nil:
			w.dev.ServeHTTP(rw, r)
		default:
			static.ServeHTTP(rw, r)
		}
	})
}

// webWrites names the "METHOD /path" writes the web mux serves besides the
// read-only GETs, each with the handler the web mux uses: the web variants
// require a state token. There is deliberately no withdraw or undo route.
var webWrites = map[string]func(*Web) http.HandlerFunc{
	"POST /v1/restart":       func(w *Web) http.HandlerFunc { return w.host.RestartWrite },
	"POST /v1/client-errors": func(w *Web) http.HandlerFunc { return w.clientErrorReport },
}

// authHost lets the web login gate ask the server what it cannot know.
type authHost struct{ w *Web }

func (h authHost) IsWebRead(r *http.Request) bool { return h.w.isRead(r) }
func (h authHost) CloseLogins(ids ...string)      { h.w.streams.closeLogins(ids...) }

// removeLegacyWebToken deletes the shared token file an older version left in the state dir.
func removeLegacyWebToken(stateDir string) error {
	err := os.Remove(filepath.Join(stateDir, "web-token"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
