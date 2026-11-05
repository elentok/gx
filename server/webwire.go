package server

import (
	"net/http"

	"github.com/elentok/gx/server/webhttp"
	"github.com/elentok/gx/server/wire"
)

// WebOnlyRoute is a route the web mux serves and the socket does not.
type WebOnlyRoute = webhttp.OnlyRoute

// WebPublicGetPatterns names every web-only "GET /path" the web mux serves.
func WebPublicGetPatterns() []string { return webhttp.PublicGetPatterns(subsystemRoutes(nil)) }

// WebOnlyRoutes returns a copy of the web-only route list.
func WebOnlyRoutes() []WebOnlyRoute { return webhttp.OnlyRoutes(subsystemRoutes(nil)) }

// WebWritePatterns names every "METHOD /path" write the web mux serves.
func WebWritePatterns() []string { return webhttp.WritePatterns(subsystemRoutes(nil)) }

// WebURL is the web listener's base URL, or "" when it is off or failed to bind.
func (s *Server) WebURL() string { return s.web.URL() }

// webHost is what the web listener asks the server for. It is its own type so
// these methods stay off Server.
type webHost struct{ s *Server }

func (h webHost) Settings() *Settings                                     { return h.s.settings.Load() }
func (h webHost) Routes() []wire.Route                                    { return h.s.routes }
func (h webHost) Snapshot(w http.ResponseWriter, r *http.Request)         { h.s.snapshot(w, r) }
func (h webHost) RestartCheckRead(w http.ResponseWriter, r *http.Request) { h.s.restarter.ServeCheck(w, r) }
func (h webHost) RestartWrite(w http.ResponseWriter, r *http.Request)     { h.s.restarter.ServeWrite(w, r) }
func (h webHost) StreamEvents(w http.ResponseWriter, r *http.Request)     { h.s.streamEvents(w, r) }

func (s *Server) newWeb() *webhttp.Web {
	return webhttp.New(webhttp.Config{
		Addr:         s.cfg.WebAddr,
		DevURL:       s.cfg.WebDevURL,
		StateDir:     s.cfg.StateDir,
		Clock:        s.cfg.Clock,
		InterfaceIPs: s.cfg.WebInterfaceIPs,
	}, s.log, webHost{s})
}
