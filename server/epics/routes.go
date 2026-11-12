package epics

import (
	"encoding/json"
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
		read("GET /v1/epics/archived", (*Epics).ArchivedList),
		read("GET /v1/epics/archived/detail", (*Epics).ArchivedDetail),
		{Pattern: "POST /v1/epics/mark-seen", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) {
			var req wire.EpicSeenRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := get().MarkSeen(req.Address); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			wire.WriteJSON(w, struct{}{})
		}},
	}
}
