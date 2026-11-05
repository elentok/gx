package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/wire"
)

// OnlyRoute is a route the web mux serves and the socket does not.
type OnlyRoute struct {
	Pattern string // "METHOD /path"
	Public  bool   // served without the web login cookie
}

// onlyRoutes is the one list of web-only routes (ADR 0028). Each is also a
// key of webWrites; a test checks the two against the socket routes.
var onlyRoutes = []OnlyRoute{
	{Pattern: "POST /v1/client-errors", Public: true},
}

// PublicGetPatterns names every web-only "GET /path" the web mux serves.
func PublicGetPatterns(routes []wire.Route) []string {
	return patternsOf(publicGetRoutes(routes))
}

// OnlyRoutes returns a copy of the web-only route list.
func OnlyRoutes(routes []wire.Route) []OnlyRoute {
	out := slices.Clone(onlyRoutes)
	for _, r := range routes {
		if r.Handler == nil && (r.Web == wire.WebWrite || r.Web == wire.WebPublic) {
			out = append(out, OnlyRoute{Pattern: r.Pattern, Public: r.Web == wire.WebPublic})
		}
	}
	return out
}

// WritePatterns names every "METHOD /path" write the web mux serves.
func WritePatterns(routes []wire.Route) []string {
	out := make([]string, 0, len(webWrites))
	for p := range webWrites {
		out = append(out, p)
	}
	return append(out, patternsOf(writeRoutes(routes))...)
}

// routesWithMode returns the routes whose web mode is one of modes.
func routesWithMode(routes []wire.Route, modes ...wire.WebMode) []wire.Route {
	var out []wire.Route
	for _, r := range routes {
		if slices.Contains(modes, r.Web) {
			out = append(out, r)
		}
	}
	return out
}

// FindRoute returns the route with this pattern and one of the web modes.
func FindRoute(routes []wire.Route, pattern string, modes ...wire.WebMode) (wire.Route, bool) {
	for _, r := range routesWithMode(routes, modes...) {
		if r.Pattern == pattern {
			return r, true
		}
	}
	return wire.Route{}, false
}

// serveRoute is the handler the web mux uses for a route.
func serveRoute(r wire.Route) http.HandlerFunc {
	if r.Web == wire.WebWrite && r.WebHandler != nil || r.Web == wire.WebPublic {
		return r.WebHandler
	}
	return r.Handler
}

func patternsOf(routes []wire.Route) []string {
	out := make([]string, len(routes))
	for i, r := range routes {
		out[i] = r.Pattern
	}
	return out
}

// isGet reports whether the route's pattern is a GET.
func isGet(r wire.Route) bool { return strings.HasPrefix(r.Pattern, "GET ") }

// writeRoutes are the moved routes the web mux serves as writes.
func writeRoutes(routes []wire.Route) []wire.Route {
	return slices.DeleteFunc(routesWithMode(routes, wire.WebWrite, wire.WebPublic), isGet)
}

// publicGetRoutes are the moved public GETs the web mux serves.
func publicGetRoutes(routes []wire.Route) []wire.Route {
	return slices.DeleteFunc(routesWithMode(routes, wire.WebPublic), func(r wire.Route) bool { return !isGet(r) })
}

// isRead reports whether the web mux serves r as a read. HEAD counts only
// for the public GET routes and the page and assets, never for /v1/ reads: the
// event stream would block on it.
func (w *Web) isRead(r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	if r.Method != http.MethodHead {
		return false
	}
	_, public := FindRoute(w.host.Routes(), "GET "+r.URL.Path, wire.WebPublic)
	return public || !strings.HasPrefix(r.URL.Path, "/v1/")
}

func (w *Web) isPublicRoute(r *http.Request) bool {
	method := r.Method
	if w.isRead(r) {
		method = http.MethodGet
	}
	key := method + " " + r.URL.Path
	for _, route := range onlyRoutes {
		if route.Public && route.Pattern == key {
			return true
		}
	}
	_, ok := FindRoute(w.host.Routes(), key, wire.WebPublic)
	return ok
}

// servePublicFirst sends public web-only routes straight to the mux, ahead of
// the login check; everything else goes through protected.
func (w *Web) servePublicFirst(mux, protected http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if w.isPublicRoute(r) {
			mux.ServeHTTP(rw, r)
			return
		}
		protected.ServeHTTP(rw, r)
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// clientErrorReport logs one warn line per accepted report. It is public, so
// it reads only the four body fields: never a cookie or other header.
func (w *Web) clientErrorReport(rw http.ResponseWriter, r *http.Request) {
	refuse := func(reason string) { writeJSON(rw, clientErrorResult{Refused: true, Reason: reason}) }
	ip := webauth.RemoteIP(r)
	if !w.errs.allow(ip, clientErrorRateLimit, clientErrorWindow) {
		refuse("rate-limited")
		return
	}
	var body clientErrorBody
	r.Body = http.MaxBytesReader(rw, r.Body, clientErrorMaxBody)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			refuse("too-large")
			return
		}
		http.Error(rw, "bad JSON", http.StatusBadRequest)
		return
	}
	// The page strips the Setup code from the hash query; cut it here too, so a client that does not never leaks it.
	if i := strings.Index(body.URL, "#"); i >= 0 {
		if q := strings.Index(body.URL[i:], "?"); q >= 0 {
			body.URL = body.URL[:i+q]
		}
	}
	w.log.Warn("client error", "message", body.Message, "stack", body.Stack, "page_url", body.URL, "user_agent", body.UserAgent, "ip", ip)
	writeJSON(rw, clientErrorResult{})
}
