package server

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
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

	"github.com/elentok/gx/webui"
)

// bindWeb opens the web listener. A failure is recorded for the handshake and
// never stops the server. New calls it once, before anything can read s.web or
// s.webErr, so the writes need no lock.
func (s *Server) bindWeb() {
	if s.cfg.WebAddr == "" {
		return
	}
	if err := removeLegacyWebToken(s.cfg.StateDir); err != nil {
		s.log.Warn("could not delete the old web token file", "err", err)
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
	auth, err := OpenWebAuth(s.cfg.StateDir, s.webNow)
	if err != nil {
		s.webErr = "web auth: " + err.Error()
		s.log.Error("web auth unavailable", "err", err)
		return
	}
	rawLn, err := net.Listen("tcp", s.cfg.WebAddr)
	if err != nil {
		s.webErr = err.Error()
		s.log.Error("web listener failed to bind", "addr", s.cfg.WebAddr, "err", err)
		return
	}
	var listenIP net.IP
	if a, ok := rawLn.Addr().(*net.TCPAddr); ok && !a.IP.IsUnspecified() {
		listenIP = a.IP
	}
	now := s.cfg.Clock
	if now == nil {
		now = time.Now
	}
	certs, err := newWebCerts(&webCerts{
		stateDir: s.cfg.StateDir,
		listenIP: listenIP,
		now:      now,
		hosts:    func() []string { return s.settings.Load().WebAllowedHosts },
		ifaceIPs: s.interfaceIPs,
		log:      s.log,
	})
	if err != nil {
		rawLn.Close()
		s.webErr = "web tls: " + err.Error()
		s.log.Error("web tls unavailable", "err", err)
		return
	}
	ln := newSniffListener(newSourceIPListener(rawLn, s.log, time.Now), &tls.Config{GetCertificate: certs.getCertificate, MinVersion: tls.VersionTLS12})
	s.web, s.webListenIP, s.webCerts = ln, listenIP, certs
	s.webAuth, s.webLoginCookie = auth, loginCookieName(ln)
	mux := s.webHandler()
	protected := s.requireWebLogin(mux, s.denyWeb(mux))
	allowed := func() []string { return s.settings.Load().WebAllowedHosts }
	s.webHTTP = &http.Server{Handler: requireHost(allowed, redirectPlainHTTP(servePublicFirst(mux, protected)))}
}

// WebURL is the web listener's base URL, or "" when it is off or failed to bind.
func (s *Server) WebURL() string {
	if s.web == nil {
		return ""
	}
	_, port, _ := net.SplitHostPort(s.web.Addr().String())
	return webBaseURL(s.settings.Load().WebAllowedHosts, s.webCerts.Root(), s.webListenIP, port, s.interfaceIPs())
}

// interfaceIPs is the one place the machine's interfaces are read.
func (s *Server) interfaceIPs() []net.IP {
	if s.cfg.WebInterfaceIPs != nil {
		return s.cfg.WebInterfaceIPs()
	}
	return interfaceIPs()
}

// webHandler is the web mux: the snapshot, the event stream and the embedded
// assets, plus the allowlisted writes. Every other request is a 404.
func (s *Server) webHandler() http.Handler {
	static := serveAsset(webui.Assets())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			if reason := checkWriteRequest(r); reason != "" {
				http.Error(w, reason, http.StatusForbidden)
				return
			}
			if handler, ok := webWrites[r.Method+" "+r.URL.Path]; ok {
				handler(s)(w, r)
				return
			}
			http.NotFound(w, r)
			return
		}
		if handler, ok := webPublicGets["GET "+r.URL.Path]; ok {
			handler(s)(w, r)
			return
		}
		switch {
		case r.URL.Path == "/v1/snapshot":
			s.snapshot(w, r)
		case r.URL.Path == "/v1/tickets/detail":
			s.detail(w, r)
		case r.URL.Path == "/v1/sessions":
			s.sessionsRead(w, r)
		case r.URL.Path == "/v1/sessions/chat":
			s.sessionChatRead(w, r)
		case r.URL.Path == "/v1/sessions/screen":
			s.sessionScreenRead(w, r)
		case r.URL.Path == "/v1/epics":
			s.epicList(w, r)
		case r.URL.Path == "/v1/epics/detail":
			s.epicDetail(w, r)
		case r.URL.Path == "/v1/projects/worktrees":
			s.projectWorktrees(w, r)
		case r.URL.Path == "/v1/stats":
			s.stats(w, r)
		case r.URL.Path == "/v1/checks/status":
			s.checksStatusRead(w, r)
		case r.URL.Path == "/v1/health":
			s.healthRead(w, r)
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

// webWrites names the "METHOD /path" writes the web mux serves besides the
// read-only GETs, each with the handler the web mux uses: the web variants
// require a state token. There is deliberately no withdraw or undo route.
var webWrites = map[string]func(*Server) http.HandlerFunc{
	"POST /v1/tickets/answer":     func(s *Server) http.HandlerFunc { return s.queueWriteWeb(s.ticketAnswer) },
	"POST /v1/tickets/resume":     func(s *Server) http.HandlerFunc { return repairWriteWeb(s.repairResume) },
	"POST /v1/tickets/unpark":     func(s *Server) http.HandlerFunc { return repairWriteWeb(s.repairResume) },
	"POST /v1/tickets/retry":      func(s *Server) http.HandlerFunc { return s.retryWrite(true) },
	"POST /v1/tickets/merge":      func(s *Server) http.HandlerFunc { return repairWriteWeb(s.mergeInQueue) },
	"POST /v1/tickets/start-over": func(s *Server) http.HandlerFunc { return repairWriteWeb(s.startOverInQueue) },
	"POST /v1/queue/add":          func(s *Server) http.HandlerFunc { return s.queueWriteWeb(s.queueAdd) },
	"POST /v1/epics/merge":        func(s *Server) http.HandlerFunc { return s.queueWriteWeb(s.epicMerge) },
	"POST /v1/epics/stash-merge":  func(s *Server) http.HandlerFunc { return s.queueWriteWeb(s.epicStashMerge) },
	"POST /v1/agents/answer":      func(s *Server) http.HandlerFunc { return s.queueWrite(s.agentAnswerWeb) },
	"POST /v1/sessions/stop":      func(s *Server) http.HandlerFunc { return s.sessionWriteDesignWeb(s.stopSession) },
	"POST /v1/sessions/resume":    func(s *Server) http.HandlerFunc { return s.sessionWriteDesignWeb(s.resumeSession) },
	"POST /v1/sessions/prompt":    func(s *Server) http.HandlerFunc { return s.sessionPromptWeb() },
	"POST /v1/sessions/create":    func(s *Server) http.HandlerFunc { return s.sessionCreateWeb },
	"POST /v1/sessions/interrupt": func(s *Server) http.HandlerFunc {
		return s.sessionWriteDesignWeb(s.interruptSession)
	},
	"POST /v1/sessions/auto-advance": func(s *Server) http.HandlerFunc { return s.sessionAutoAdvanceWeb() },
	"POST /v1/sessions/auto-advance/cancel": func(s *Server) http.HandlerFunc {
		return s.sessionWriteDesignWeb(s.cancelAutoAdvanceRoute)
	},
	"POST /v1/designs/open":      func(s *Server) http.HandlerFunc { return s.designOpenWeb() },
	"POST /v1/designs/delete":    func(s *Server) http.HandlerFunc { return s.designDeleteWrite },
	"POST /v1/tickets/fix-agent": func(s *Server) http.HandlerFunc { return s.fixAgentWeb() },
	"POST /v1/sessions/focus":    func(s *Server) http.HandlerFunc { return s.sessionFocusWriteWeb },
	"POST /v1/afk":               func(s *Server) http.HandlerFunc { return s.afkWrite },
	"POST /v1/checks/save":       func(s *Server) http.HandlerFunc { return s.checksSaveWrite },
	"POST /v1/disk/clean":        func(s *Server) http.HandlerFunc { return s.diskCleanWrite },
	"POST /v1/client-errors":     func(s *Server) http.HandlerFunc { return s.clientErrorReport },

	"POST " + webAuthPathSetupStart:  func(s *Server) http.HandlerFunc { return s.webSetupStart },
	"POST " + webAuthPathSetupFinish: func(s *Server) http.HandlerFunc { return s.webSetupFinish },
	"POST " + webAuthPathLogin:       func(s *Server) http.HandlerFunc { return s.webLoginWrite },
	"POST " + webAuthPathAddAuth:     func(s *Server) http.HandlerFunc { return s.webAddAuthenticator },
	"POST " + webAuthPathSignOut:     func(s *Server) http.HandlerFunc { return s.webSignOut },
	"POST " + webAuthPathLogins:      func(s *Server) http.HandlerFunc { return s.webLoginsList },
	"POST " + webAuthPathLoginRename: func(s *Server) http.HandlerFunc { return s.webLoginRename },
	"POST " + webAuthPathLoginRevoke: func(s *Server) http.HandlerFunc { return s.webLoginRevoke },
	"POST " + webAuthPathPassword:    func(s *Server) http.HandlerFunc { return s.webChangePassword },
	"POST " + webAuthPathResetBegin:  func(s *Server) http.HandlerFunc { return s.webAuthenticatorBegin },
	"POST " + webAuthPathResetFinish: func(s *Server) http.HandlerFunc { return s.webAuthenticatorFinish },
}

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

// serveAsset serves index.html at "/" only, and the file at any other path.
// A path with no file is a 404, so a stale page fails loudly instead of
// loading HTML as a script. The app uses a hash router, so it needs no fallback.
func serveAsset(assets fs.FS) http.Handler {
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
