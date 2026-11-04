package projects

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// Routes lists the project, migration and checks endpoints. The getters are
// called per request, so a route can be listed before its Server is built.
func Routes(reg func() *Registry, mig func() *Migrator, chk func() *Checks) []wire.Route {
	return []wire.Route{
		{Pattern: "GET /v1/projects/worktrees", Web: wire.WebRead, Handler: func(w http.ResponseWriter, r *http.Request) { reg().worktrees(w, r) }},
		{Pattern: "POST /v1/projects/add", Handler: func(w http.ResponseWriter, r *http.Request) {
			handle(w, r, reg().Add)
		}},
		{Pattern: "POST /v1/projects/remove", Handler: func(w http.ResponseWriter, r *http.Request) {
			handle(w, r, reg().Remove)
		}},
		{Pattern: "POST /v1/projects/set-path", Handler: func(w http.ResponseWriter, r *http.Request) {
			handle(w, r, reg().SetPath)
		}},
		{Pattern: "POST /v1/projects/retry-migration", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) {
			handle(w, r, func(req wire.RetryMigrationRequest) (wire.RetryMigrationResult, error) {
				return mig().Retry(req.Project), nil
			})
		}},
		{Pattern: "GET /v1/checks/status", Web: wire.WebRead, Handler: func(w http.ResponseWriter, r *http.Request) { chk().statusRead(w, r) }},
		{Pattern: "POST /v1/checks/save", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) {
			handle(w, r, chk().Save)
		}},
	}
}

// handle decodes the JSON body into Req, runs do and writes its result.
func handle[Req, Res any](w http.ResponseWriter, r *http.Request, do func(Req) (Res, error)) {
	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

func (p *Registry) worktrees(w http.ResponseWriter, _ *http.Request) {
	list, err := p.ListWorktrees()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, list)
}

func (c *Checks) statusRead(w http.ResponseWriter, r *http.Request) {
	st, err := c.Status(r.URL.Query().Get("project"))
	if err != nil {
		code := http.StatusInternalServerError
		var ae *tickets.AddressError
		if errors.As(err, &ae) {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	wire.WriteJSON(w, st)
}
