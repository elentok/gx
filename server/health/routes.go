package health

import (
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// Routes lists the health report and CPU spike endpoints. The getter is called
// per request, so a route can be listed before its Health is built.
func Routes(get func() *Health) []wire.Route {
	return []wire.Route{
		{Pattern: "GET /v1/health", Web: wire.WebRead, Handler: func(w http.ResponseWriter, r *http.Request) { get().healthRead(w, r) }},
		{Pattern: "POST /v1/cpu-spike", Handler: func(w http.ResponseWriter, r *http.Request) { get().cpuSpikeWrite(w, r) }},
	}
}
