package web

import (
	"encoding/json"
	"errors"
	"io"
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

// Routes are the web listener's own endpoints: the socket's web-login admin
// routes, and the browser's reports only the web mux serves (ADR 0028).
func Routes(web func() *Web) []wire.Route {
	call := func(f func(*Web, http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(rw http.ResponseWriter, r *http.Request) { f(web(), rw, r) }
	}
	return []wire.Route{
		{Pattern: "GET /v1/web/info", Handler: call((*Web).Info)},
		{Pattern: "POST /v1/web/new-root", Handler: call((*Web).NewRoot)},
		{Pattern: "POST /v1/web/reset", Handler: call((*Web).Reset)},
		{Pattern: "POST /v1/web/reset-authenticator", Handler: call((*Web).ResetAuthenticator)},
		{Pattern: "GET /v1/web/login-list", Handler: call((*Web).LoginList)},
		{Pattern: "POST /v1/web/login-revoke", Handler: call((*Web).LoginRevoke)},
		{Pattern: "POST /v1/web/set-password", Handler: call((*Web).SetPassword)},
		{Pattern: "POST /v1/client-errors", Web: wire.WebPublic, WebHandler: call((*Web).clientErrorReport)},
		{Pattern: "POST /v1/web-performance", Web: wire.WebWrite, WebHandler: call((*Web).webPerfReport)},
		{Pattern: "POST /v1/broken-rules", Web: wire.WebWrite, WebHandler: call((*Web).brokenRuleReport)},
	}
}

// PublicGetPatterns names every web-only "GET /path" the web mux serves.
func PublicGetPatterns(routes []wire.Route) []string {
	return patternsOf(publicGetRoutes(routes))
}

// OnlyRoutes lists the routes the web mux serves and the socket does not.
func OnlyRoutes(routes []wire.Route) []OnlyRoute {
	var out []OnlyRoute
	for _, r := range routes {
		if r.Handler == nil && (r.Web == wire.WebWrite || r.Web == wire.WebPublic || r.Web == wire.WebReadPrefix) {
			out = append(out, OnlyRoute{Pattern: r.Pattern, Public: r.Web == wire.WebPublic})
		}
	}
	return out
}

// PrefixGetPatterns names every web-only "GET /prefix/" the web mux serves.
func PrefixGetPatterns(routes []wire.Route) []string {
	return patternsOf(routesWithMode(routes, wire.WebReadPrefix))
}

// findPrefixRoute returns the prefix route that owns r. It matches the raw
// escaped path, so an encoded separator never reaches a prefix by decoding.
func findPrefixRoute(routes []wire.Route, r *http.Request) (wire.Route, bool) {
	target := "GET " + r.URL.EscapedPath()
	for _, route := range routesWithMode(routes, wire.WebReadPrefix) {
		if strings.HasPrefix(target, route.Pattern) {
			return route, true
		}
	}
	return wire.Route{}, false
}

// WritePatterns names every "METHOD /path" write the web mux serves.
func WritePatterns(routes []wire.Route) []string {
	return patternsOf(writeRoutes(routes))
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
	if r.Web == wire.WebWrite && r.WebHandler != nil || r.Web == wire.WebPublic || r.Web == wire.WebReadPrefix {
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
	_, ok := FindRoute(w.host.Routes(), method+" "+r.URL.Path, wire.WebPublic)
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

// clientErrorReport logs one warn line per accepted report. It is public, so
// it reads only the four body fields: never a cookie or other header.
func (w *Web) clientErrorReport(rw http.ResponseWriter, r *http.Request) {
	refuse := func(reason string) { wire.WriteJSON(rw, clientErrorResult{Refused: true, Reason: reason}) }
	ip := webauth.RemoteIP(r)
	var body clientErrorBody
	if !readReport(rw, r, w.errs.allow(ip, clientErrorRateLimit, clientErrorWindow), clientErrorMaxBody, &body, refuse) {
		return
	}
	w.log.Warn("client error", "message", body.Message, "stack", body.Stack, "page_url", cleanPageURL(body.URL), "user_agent", body.UserAgent, "ip", ip)
	wire.WriteJSON(rw, clientErrorResult{})
}

// readReport is the intake shared by the browser report routes: it reads the
// body into report, or answers the request itself and returns false. allowed is
// the verdict of the route's own limiter.
func readReport(rw http.ResponseWriter, r *http.Request, allowed bool, maxBody int64, report any, refuse func(reason string)) bool {
	if !allowed {
		refuse("rate-limited")
		return false
	}
	r.Body = http.MaxBytesReader(rw, r.Body, maxBody)
	err := json.NewDecoder(r.Body).Decode(report)
	if err == nil {
		// Decode stops after the first value, so read on to catch padding.
		_, err = io.Copy(io.Discard, r.Body)
	}
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		refuse("too-large")
	case err != nil:
		http.Error(rw, "bad JSON", http.StatusBadRequest)
	}
	return err == nil
}

// cleanPageURL cuts the hash query off a reported page URL. The page strips the
// Setup code from it; cutting it here too means a client that does not never
// leaks it.
func cleanPageURL(url string) string {
	if i := strings.Index(url, "#"); i >= 0 {
		if q := strings.Index(url[i:], "?"); q >= 0 {
			return url[:i+q]
		}
	}
	return url
}
