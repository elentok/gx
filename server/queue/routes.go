package queue

import (
	"net/http"

	"github.com/elentok/gx/server/wire"
)

// Routes lists the queue, one-off, AFK and vi-mode endpoints. The getters are
// called per request, so a route can be listed before its Server is built.
func Routes(queue func() *Queue, oneOffs func() *OneOffs, afk func() *AFK, viMode func() *ViMode) []wire.Route {
	write := func(pattern string, do func(*Queue) func(wire.QueueRequest) (wire.QueueResult, error)) wire.Route {
		return wire.Route{Pattern: pattern, Handler: func(w http.ResponseWriter, r *http.Request) { Handler(do(queue()))(w, r) }}
	}
	mode := func(pattern string, do func(*Queue) func() (wire.QueueResult, error)) wire.Route {
		return wire.Route{Pattern: pattern, Handler: func(w http.ResponseWriter, r *http.Request) { modeWrite(do(queue()))(w, r) }}
	}
	add := write("POST /v1/queue/add", func(q *Queue) func(wire.QueueRequest) (wire.QueueResult, error) { return q.Add })
	add.Web = wire.WebWrite
	add.WebHandler = func(w http.ResponseWriter, r *http.Request) { HandlerWeb(queue().Add)(w, r) }
	return []wire.Route{
		{Pattern: "GET /v1/queue/items", Handler: func(w http.ResponseWriter, _ *http.Request) { wire.WriteJSON(w, queue().List()) }},
		add,
		write("POST /v1/queue/remove", func(q *Queue) func(wire.QueueRequest) (wire.QueueResult, error) { return q.Remove }),
		write("POST /v1/queue/move", func(q *Queue) func(wire.QueueRequest) (wire.QueueResult, error) { return q.Move }),
		write("POST /v1/queue/replace", func(q *Queue) func(wire.QueueRequest) (wire.QueueResult, error) { return q.Replace }),
		mode("POST /v1/queue/pause", func(q *Queue) func() (wire.QueueResult, error) { return q.Pause }),
		mode("POST /v1/queue/resume", func(q *Queue) func() (wire.QueueResult, error) { return q.Resume }),
		mode("POST /v1/queue/drain", func(q *Queue) func() (wire.QueueResult, error) { return q.Flush }),
		{Pattern: "POST /v1/oneoff", Handler: func(w http.ResponseWriter, r *http.Request) { oneOffs().handle(w, r) }},
		{Pattern: "POST /v1/afk", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { afk().write(w, r) }},
		{Pattern: "POST /v1/vimode", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { viMode().write(w, r) }},
	}
}
