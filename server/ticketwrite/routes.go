package ticketwrite

import (
	"net/http"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
)

// Routes lists the single-ticket write endpoints. The getter is called per
// request, so a route can be listed before its Writer is built. park is a
// hidden alias of pause; relaunch is the old name of retry, kept for one release.
func Routes(get func() *Writer) []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/tickets/pause", Handler: func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().PauseRequest)(rw, r) }},
		{Pattern: "POST /v1/tickets/park", Handler: func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().PauseRequest)(rw, r) }},
		{Pattern: "POST /v1/tickets/nudge", Handler: func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().Nudge)(rw, r) }},
		{Pattern: "POST /v1/tickets/relaunch", Handler: func(rw http.ResponseWriter, r *http.Request) { queue.HandlerCtx(get().Relaunch)(rw, r) }},
	}
}
