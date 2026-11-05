package ticketops

import (
	"net/http"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
)

// Routes lists the answer, ask, recommend and agent endpoints. The getter is
// called per request, so a route can be listed before its Ops is built. The
// web mux serves the answer routes through their web variants, which need a
// state token or a request ID.
func Routes(get func() *Ops) []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/tickets/recommend", Handler: func(rw http.ResponseWriter, r *http.Request) { queue.HandlerCtx(get().Recommend)(rw, r) }},
		{
			Pattern: "POST /v1/tickets/answer", Web: wire.WebWrite,
			Handler:    func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().Answer)(rw, r) },
			WebHandler: func(rw http.ResponseWriter, r *http.Request) { queue.HandlerWeb(get().Answer)(rw, r) },
		},
		{Pattern: "POST /v1/tickets/ask", Handler: func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().Ask)(rw, r) }},
		{Pattern: "POST /v1/agents/prompt", Handler: func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().AgentPrompt)(rw, r) }},
		{Pattern: "POST /v1/agents/interrupt", Handler: func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().AgentInterrupt)(rw, r) }},
		{
			Pattern: "POST /v1/agents/answer", Web: wire.WebWrite,
			Handler:    func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().AgentAnswer)(rw, r) },
			WebHandler: func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().AgentAnswerWeb)(rw, r) },
		},
	}
}
