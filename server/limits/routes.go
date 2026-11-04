package limits

import (
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// Routes lists the usage report, override and summary endpoints. The getter is
// called per request, so a route can be listed before its Limits is built.
func Routes(get func() *Limits) []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/usage/report", Handler: func(w http.ResponseWriter, r *http.Request) { get().reportWrite(w, r) }},
		{Pattern: "POST /v1/usage/override", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { get().overrideWrite(w, r) }},
		{Pattern: "GET /v1/usage/summary", Web: wire.WebRead, Handler: func(w http.ResponseWriter, r *http.Request) { get().summaryRead(w, r) }},
	}
}
