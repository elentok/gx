package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

// WebOnlyRoute is a route the web mux serves and the socket does not.
type WebOnlyRoute struct {
	Pattern string // "METHOD /path"
	Public  bool   // served without the web login cookie
}

// webOnlyRoutes is the one list of web-only routes (ADR 0028). Each is also a
// key of webWrites; a test checks the two against routeTable.
var webOnlyRoutes = []WebOnlyRoute{
	{Pattern: "POST /v1/client-errors", Public: true},
	{Pattern: "GET " + webRootProfilePath, Public: true},
	{Pattern: "GET " + webRootCerPath, Public: true},
	{Pattern: "GET " + webAuthPathState, Public: true},
	{Pattern: "POST " + webAuthPathSetupStart, Public: true},
	{Pattern: "POST " + webAuthPathSetupPassword, Public: true},
	{Pattern: "POST " + webAuthPathSetupFinish, Public: true},
	{Pattern: "POST " + webAuthPathLogin, Public: true},
	{Pattern: "POST " + webAuthPathAddAuth, Public: true},
	{Pattern: "POST " + webAuthPathSignOut},
	{Pattern: "POST " + webAuthPathLogins},
	{Pattern: "POST " + webAuthPathLoginRename},
	{Pattern: "POST " + webAuthPathLoginRevoke},
	{Pattern: "POST " + webAuthPathPassword},
	{Pattern: "POST " + webAuthPathResetBegin},
	{Pattern: "POST " + webAuthPathResetFinish},
}

// webPublicGets names the GET routes in webOnlyRoutes and their handlers.
var webPublicGets = map[string]func(*Server) http.HandlerFunc{
	"GET " + webRootProfilePath: func(s *Server) http.HandlerFunc { return s.webRootProfileRead },
	"GET " + webRootCerPath:     func(s *Server) http.HandlerFunc { return s.webRootCerRead },
	"GET " + webAuthPathState:   func(s *Server) http.HandlerFunc { return s.webAuthStateRead },
}

// WebPublicGetPatterns names every web-only "GET /path" the web mux serves.
func WebPublicGetPatterns() []string {
	out := make([]string, 0, len(webPublicGets))
	for p := range webPublicGets {
		out = append(out, p)
	}
	return out
}

// WebOnlyRoutes returns a copy of the web-only route list.
func WebOnlyRoutes() []WebOnlyRoute { return append([]WebOnlyRoute(nil), webOnlyRoutes...) }

// WebWritePatterns names every "METHOD /path" write the web mux serves.
func WebWritePatterns() []string {
	out := make([]string, 0, len(webWrites))
	for p := range webWrites {
		out = append(out, p)
	}
	return out
}

// isWebRead reports whether the web mux serves r as a read. HEAD counts only
// for the public GET routes and the page and assets, never for /v1/ reads: the
// event stream would block on it.
func isWebRead(r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	if r.Method != http.MethodHead {
		return false
	}
	_, public := webPublicGets["GET "+r.URL.Path]
	return public || !strings.HasPrefix(r.URL.Path, "/v1/")
}

func isPublicWebRoute(r *http.Request) bool {
	method := r.Method
	if isWebRead(r) {
		method = http.MethodGet
	}
	key := method + " " + r.URL.Path
	for _, route := range webOnlyRoutes {
		if route.Public && route.Pattern == key {
			return true
		}
	}
	return false
}

// servePublicFirst sends public web-only routes straight to the mux, ahead of
// the login check; everything else goes through protected.
func servePublicFirst(mux, protected http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicWebRoute(r) {
			mux.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

const (
	clientErrorMaxBody   = 16 << 10
	clientErrorRateLimit = 20
	clientErrorWindow    = time.Minute
)

// ipLimiter allows a fixed number of events per IP in each window. The zero
// value is ready to use.
type ipLimiter struct {
	mu     sync.Mutex
	counts map[string]int
	start  time.Time
}

func (l *ipLimiter) allow(ip string, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now := time.Now(); l.counts == nil || now.Sub(l.start) >= window {
		l.counts = map[string]int{}
		l.start = now
	}
	l.counts[ip]++
	return l.counts[ip] <= limit
}

type clientErrorBody struct {
	Message   string `json:"message"`
	Stack     string `json:"stack"`
	URL       string `json:"url"`
	UserAgent string `json:"user_agent"`
}

type clientErrorResult struct {
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// clientErrorReport logs one warn line per accepted report. It is public, so
// it reads only the four body fields: never a cookie or other header.
func (s *Server) clientErrorReport(w http.ResponseWriter, r *http.Request) {
	refuse := func(reason string) { writeJSON(w, clientErrorResult{Refused: true, Reason: reason}) }
	ip := remoteIP(r)
	if !s.clientErrs.allow(ip, clientErrorRateLimit, clientErrorWindow) {
		refuse("rate-limited")
		return
	}
	var body clientErrorBody
	r.Body = http.MaxBytesReader(w, r.Body, clientErrorMaxBody)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			refuse("too-large")
			return
		}
		http.Error(w, "bad JSON", http.StatusBadRequest)
		return
	}
	// The page strips the Setup code from the hash query; cut it here too, so a client that does not never leaks it.
	if i := strings.Index(body.URL, "#"); i >= 0 {
		if q := strings.Index(body.URL[i:], "?"); q >= 0 {
			body.URL = body.URL[:i+q]
		}
	}
	s.log.Warn("client error", "message", body.Message, "stack", body.Stack, "page_url", body.URL, "user_agent", body.UserAgent, "ip", ip)
	writeJSON(w, clientErrorResult{})
}
