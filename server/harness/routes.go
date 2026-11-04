package harness

import (
	"encoding/json"
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// Routes lists the Default harness endpoints. The getter is called per request,
// so a route can be listed before its Harness is built.
func Routes(h func() *Harness) []wire.Route {
	return []wire.Route{
		{Pattern: "GET /v1/settings/agents", Web: wire.WebRead, Handler: func(w http.ResponseWriter, _ *http.Request) {
			st, err := h().Status()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			wire.WriteJSON(w, st)
		}},
		{Pattern: "POST /v1/settings/agents", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) {
			var req wire.SetDefaultHarnessRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			res, err := h().SetDefault(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			wire.WriteJSON(w, res)
		}},
	}
}
