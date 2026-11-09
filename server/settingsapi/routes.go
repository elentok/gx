package settingsapi

import (
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// GlobalTarget is the `target` value that names the global config file.
const GlobalTarget = "global"

// Routes lists the settings page endpoints. pending reports the startup-only
// keys changed since the server started. The getter is called per request.
func Routes(w func() *Writer, pending func() []string) []wire.Route {
	return []wire.Route{
		{Pattern: "GET /v1/settings", Web: wire.WebRead, Handler: func(rw http.ResponseWriter, r *http.Request) {
			var t Target
			switch target := r.URL.Query().Get("target"); target {
			case GlobalTarget:
			case "":
				http.Error(rw, "target is required: global or a project name", http.StatusBadRequest)
				return
			default:
				t.Project = target
				if _, err := w().ProjectDir(target); err != nil {
					http.Error(rw, err.Error(), http.StatusNotFound)
					return
				}
			}
			resp, err := w().Read(t, pending())
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}
			wire.WriteJSON(rw, resp)
		}},
	}
}
