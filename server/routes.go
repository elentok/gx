package server

import (
	"crypto/x509"
	"net/http"
	"slices"
	"strings"

	"github.com/elentok/gx/server/disk"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/webauth"
	"github.com/elentok/gx/server/webtls"
	"github.com/elentok/gx/server/wire"
)

// subsystemRoutes gathers the Routes() of every subsystem that has moved out
// of the root. A move ticket adds its subsystem here and drops its entries
// from routeTable, webWrites and the web read switch. It is also called with
// a nil Server to list patterns, so the handlers must not need one yet.
// A var so the assembly test can swap in a fake list.
var subsystemRoutes = func(s *Server) []wire.Route {
	return slices.Concat(
		webtls.Routes(func() *x509.Certificate { return s.webCerts.Root() }),
		webauth.Routes(func() *webauth.Gate { return s.webAuth }),
		settings.Routes(func() *settings.Holder { return s.Settings() }),
		index.Routes(func() *index.Index { return s.idx }, func() <-chan struct{} { return s.migrator.Done() }),
		projects.Routes(func() *projects.Registry { return s.projects }, func() *projects.Migrator { return s.migrator }, func() *projects.Checks { return s.checks }),
		queue.Routes(func() *queue.Queue { return s.queue }, func() *queue.OneOffs { return s.oneOffs }, func() *queue.AFK { return s.afk }, func() *queue.ViMode { return s.viMode }),
		harnesschoice.Routes(func() *harnesschoice.Harness { return s.harness }),
		disk.Routes(func() *disk.Disk { return s.disks }),
		limits.Routes(func() *limits.Limits { return s.limits }),
	)
}

// registerRoutes puts each route that has a socket handler on mux.
func registerRoutes(mux *http.ServeMux, routes []wire.Route) {
	for _, r := range routes {
		if r.Handler != nil {
			mux.HandleFunc(r.Pattern, r.Handler)
		}
	}
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

// findRoute returns the route with this pattern and one of the web modes.
func findRoute(routes []wire.Route, pattern string, modes ...wire.WebMode) (wire.Route, bool) {
	for _, r := range routesWithMode(routes, modes...) {
		if r.Pattern == pattern {
			return r, true
		}
	}
	return wire.Route{}, false
}

// webServe is the handler the web mux uses for a route.
func webServe(r wire.Route) http.HandlerFunc {
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

// webWriteRoutes are the moved routes the web mux serves as writes.
func webWriteRoutes(routes []wire.Route) []wire.Route {
	return slices.DeleteFunc(routesWithMode(routes, wire.WebWrite, wire.WebPublic), isGet)
}

// webPublicGetRoutes are the moved public GETs the web mux serves.
func webPublicGetRoutes(routes []wire.Route) []wire.Route {
	return slices.DeleteFunc(routesWithMode(routes, wire.WebPublic), func(r wire.Route) bool { return !isGet(r) })
}
