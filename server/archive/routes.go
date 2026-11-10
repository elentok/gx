package archive

import (
	"encoding/json"
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// Routes lists the archive endpoint. The getter is called per request, so a
// route can be listed before its Archive is built.
func Routes(a func() *Archive) []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/epics/archive", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) {
			var req wire.ArchiveRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			res, err := a().Epics(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			wire.WriteJSON(w, res)
		}},
	}
}
