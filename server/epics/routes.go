package epics

import (
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// Routes lists the ticket-detail and epic read endpoints. The getter is called
// per request, so a route can be listed before its Epics is built.
func Routes(get func() *Epics) []wire.Route {
	read := func(pattern string, do func(*Epics, http.ResponseWriter, *http.Request)) wire.Route {
		return wire.Route{
			Pattern: pattern,
			Web:     wire.WebRead,
			Handler: func(w http.ResponseWriter, r *http.Request) { do(get(), w, r) },
		}
	}
	return []wire.Route{
		read("GET /v1/tickets/detail", (*Epics).Detail),
		read("GET /v1/epics", (*Epics).EpicList),
		read("GET /v1/epics/detail", (*Epics).EpicDetail),
	}
}
