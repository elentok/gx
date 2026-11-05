package ticketops

import (
	"net/http"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
)

// repairRoute is a repair write: the socket takes a plain request, the web mux
// one with a state token.
func repairRoute(verb string, get func() *Ops, do func(*Ops, wire.RepairRequest, repairCtx) (wire.RepairResult, error)) wire.Route {
	bind := func(o *Ops) func(wire.RepairRequest, repairCtx) (wire.RepairResult, error) {
		return func(req wire.RepairRequest, rc repairCtx) (wire.RepairResult, error) { return do(o, req, rc) }
	}
	return wire.Route{
		Pattern: "POST /v1/tickets/" + verb, Web: wire.WebWrite,
		Handler:    func(rw http.ResponseWriter, r *http.Request) { repairWrite(bind(get()))(rw, r) },
		WebHandler: func(rw http.ResponseWriter, r *http.Request) { repairWriteWeb(bind(get()))(rw, r) },
	}
}

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
		repairRoute("merge", get, (*Ops).mergeInQueue),
		repairRoute("start-over", get, (*Ops).startOverInQueue),
		repairRoute("resume", get, (*Ops).repairResume),
		repairRoute("unpark", get, (*Ops).repairResume), // hidden alias of resume
		{Pattern: "POST /v1/tickets/verify", Handler: func(rw http.ResponseWriter, r *http.Request) { repairWrite(get().repairVerify)(rw, r) }},
		{Pattern: "POST /v1/tickets/ask", Handler: func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().Ask)(rw, r) }},
		{Pattern: "POST /v1/tickets/unmute", Handler: func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().Unmute)(rw, r) }},
		{Pattern: "POST /v1/tickets/cancel", Handler: func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().Cancel)(rw, r) }},
		{
			Pattern: "POST /v1/tickets/retry", Web: wire.WebWrite,
			Handler:    func(rw http.ResponseWriter, r *http.Request) { get().Retry(false)(rw, r) },
			WebHandler: func(rw http.ResponseWriter, r *http.Request) { get().Retry(true)(rw, r) },
		},
		{Pattern: "POST /v1/agents/prompt", Handler: func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().AgentPrompt)(rw, r) }},
		{Pattern: "POST /v1/agents/interrupt", Handler: func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().AgentInterrupt)(rw, r) }},
		{
			Pattern: "POST /v1/agents/answer", Web: wire.WebWrite,
			Handler:    func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().AgentAnswer)(rw, r) },
			WebHandler: func(rw http.ResponseWriter, r *http.Request) { queue.Handler(get().AgentAnswerWeb)(rw, r) },
		},
	}
}
