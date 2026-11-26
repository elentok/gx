package server

import (
	"github.com/elentok/gx/server/web"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/webperf"
)

// WebOnlyRoute is a route the web mux serves and the socket does not.
type WebOnlyRoute = web.OnlyRoute

// WebPublicGetPatterns names every web-only "GET /path" the web mux serves.
func WebPublicGetPatterns() []string { return web.PublicGetPatterns(assembleRoutes(nil)) }

// WebPrefixGetPatterns names every web-only "GET /prefix/" the web mux serves.
func WebPrefixGetPatterns() []string { return web.PrefixGetPatterns(assembleRoutes(nil)) }

// WebOnlyRoutes lists the routes the web mux serves and the socket does not.
func WebOnlyRoutes() []WebOnlyRoute { return web.OnlyRoutes(assembleRoutes(nil)) }

// WebWritePatterns names every "METHOD /path" write the web mux serves.
func WebWritePatterns() []string { return web.WritePatterns(assembleRoutes(nil)) }

// WebURL is the web listener's base URL, or "" when it is off or failed to bind.
func (s *Server) WebURL() string { return s.web.URL() }

// webHost is what the web listener asks the server for. It is its own type so
// these methods stay off Server.
type webHost struct{ s *Server }

func (h webHost) Settings() *Settings     { return h.s.settings.Load() }
func (h webHost) Routes() []wire.Route    { return h.s.routes }
func (h webHost) WebPerf() *webperf.Store { return h.s.webPerf }
func (h webHost) PushKey() string         { return h.s.vapid.Public() }

func (h webHost) FileBrokenRule(r ops.BrokenRuleReport) error {
	return h.s.recovery.FileBrokenRule(r)
}

func (s *Server) newWeb() *web.Web {
	return web.New(web.Config{
		Addr:         s.cfg.WebAddr,
		StateDir:     s.cfg.StateDir,
		Clock:        s.cfg.Clock,
		InterfaceIPs: s.cfg.WebInterfaceIPs,
	}, s.log, webHost{s})
}
