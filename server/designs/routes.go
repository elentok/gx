package designs

import (
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// Routes lists the designs open, delete and worktree writes. Open and delete
// are also web writes; open's web variant needs the state token. The getter is
// called per request, so a route can be listed before its Designs is built.
func Routes(get func() *Designs) []wire.Route {
	return []wire.Route{
		{
			Pattern:    "POST /v1/designs/open",
			Web:        wire.WebWrite,
			Handler:    func(w http.ResponseWriter, r *http.Request) { get().openHandler(false)(w, r) },
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().openHandler(true)(w, r) },
		},
		{
			Pattern: "POST /v1/designs/delete",
			Web:     wire.WebWrite,
			Handler: func(w http.ResponseWriter, r *http.Request) { get().deleteWrite(w, r) },
		},
		{
			Pattern: "GET /v1/designs/drafts",
			Web:     wire.WebRead,
			Handler: func(w http.ResponseWriter, r *http.Request) { get().draftList(w, r) },
		},
		{
			Pattern: "GET /v1/designs/drafts/read",
			Web:     wire.WebRead,
			Handler: func(w http.ResponseWriter, r *http.Request) { get().draftRead(w, r) },
		},
		{
			Pattern: "POST /v1/designs/drafts/save",
			Web:     wire.WebWrite,
			Handler: func(w http.ResponseWriter, r *http.Request) { get().draftSave(w, r) },
		},
		{
			Pattern: "POST /v1/designs/drafts/delete",
			Web:     wire.WebWrite,
			Handler: func(w http.ResponseWriter, r *http.Request) { get().draftDelete(w, r) },
		},
		{
			Pattern: "POST /v1/designs/worktree",
			Handler: func(w http.ResponseWriter, r *http.Request) { get().worktreeWrite(w, r) },
		},
	}
}
