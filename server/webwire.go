package server

import (
	"github.com/elentok/gx/server/web"
	"github.com/elentok/gx/server/wire"
)

// WebOnlyRoute is a route the web mux serves and the socket does not.
type WebOnlyRoute = web.OnlyRoute

// WebPublicGetPatterns names every web-only "GET /path" the web mux serves.
func WebPublicGetPatterns() []string { return web.PublicGetPatterns(assembleRoutes(nil)) }

// WebOnlyRoutes lists the routes the web mux serves and the socket does not.
func WebOnlyRoutes() []WebOnlyRoute { return web.OnlyRoutes(assembleRoutes(nil)) }

// WebWritePatterns names every "METHOD /path" write the web mux serves.
func WebWritePatterns() []string { return web.WritePatterns(assembleRoutes(nil)) }

// WebURL is the web listener's base URL, or "" when it is off or failed to bind.
func (s *Server) WebURL() string { return s.web.URL() }

// WebUI is the server's web listener, for tests that switch what it serves
// (UseViteProxy, UseBuiltUI). A function, not a method, to keep Server small.
func WebUI(s *Server) *web.Web { return s.web }

// webHost is what the web listener asks the server for. It is its own type so
// these methods stay off Server.
type webHost struct{ s *Server }

func (h webHost) Settings() *Settings  { return h.s.settings.Load() }
func (h webHost) Routes() []wire.Route { return h.s.routes }

func (s *Server) newWeb() *web.Web {
	return web.New(web.Config{
		Addr:         s.cfg.WebAddr,
		StateDir:     s.cfg.StateDir,
		Clock:        s.cfg.Clock,
		InterfaceIPs: s.cfg.WebInterfaceIPs,
	}, s.log, webHost{s})
}
