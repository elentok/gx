package merge

import (
	"net/http"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
)

// Routes lists the epic merge and stash-merge writes, each with its web
// variant. The getters are called per request, so a route can be listed before
// its Merger and Landing are built.
func Routes(merger func() *Merger, landing func() *Landing) []wire.Route {
	write := func(pattern string, do func() func(wire.QueueRequest) (wire.QueueResult, error)) wire.Route {
		return wire.Route{
			Pattern:    pattern,
			Web:        wire.WebWrite,
			Handler:    func(w http.ResponseWriter, r *http.Request) { queue.Handler(do())(w, r) },
			WebHandler: func(w http.ResponseWriter, r *http.Request) { queue.HandlerWeb(do())(w, r) },
		}
	}
	return []wire.Route{
		write("POST /v1/epics/merge", func() func(wire.QueueRequest) (wire.QueueResult, error) { return merger().EpicMerge }),
		write("POST /v1/epics/stash-merge", func() func(wire.QueueRequest) (wire.QueueResult, error) { return landing().EpicStashMerge }),
	}
}
