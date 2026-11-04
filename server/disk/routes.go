package disk

import (
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// Routes lists the disk status and clean endpoints. The getter is called per
// request, so a route can be listed before its Disk is built.
func Routes(disk func() *Disk) []wire.Route {
	return []wire.Route{
		{Pattern: "GET /v1/disk/status", Handler: func(w http.ResponseWriter, r *http.Request) { disk().statusRead(w, r) }},
		{Pattern: "POST /v1/disk/clean", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { disk().cleanWrite(w, r) }},
	}
}
