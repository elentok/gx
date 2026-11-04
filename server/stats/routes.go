package stats

import (
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// Routes lists the stats endpoint. The getter is called per request, so a
// route can be listed before its Stats is built.
func Routes(get func() *Stats) []wire.Route {
	return []wire.Route{
		{Pattern: "GET /v1/stats", Web: wire.WebRead, Handler: func(w http.ResponseWriter, r *http.Request) { get().read(w, r) }},
	}
}
