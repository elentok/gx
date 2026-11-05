package sessions

import (
	"net/http"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
)

// Routes lists the session endpoints the package owns. The getters are called
// per request, so a route can be listed before its Manager is built.
func Routes(mg func() *Manager, rs func() *Resumer, q func() *queue.Queue) []wire.Route {
	m := func(h func(*Manager, http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { h(mg(), w, r) }
	}
	res := func(h func(*Resumer, http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { h(rs(), w, r) }
	}
	// do picks the session action off the Manager or Resumer at request time.
	write := func(do func() func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { SessionWrite(do())(w, r) }
	}
	agentWeb := func(do func() func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { mg().SessionWriteAgentWeb(do())(w, r) }
	}
	designWeb := func(do func() func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { mg().SessionWriteDesignWeb(do())(w, r) }
	}
	stop := func() func(wire.SessionRequest) (wire.SessionResult, error) { return mg().StopSession }
	remove := func() func(wire.SessionRequest) (wire.SessionResult, error) { return mg().RemoveSession }
	interrupt := func() func(wire.SessionRequest) (wire.SessionResult, error) { return mg().InterruptSession }
	resume := func() func(wire.SessionRequest) (wire.SessionResult, error) { return rs().ResumeSession }
	return []wire.Route{
		{Pattern: "GET /v1/sessions", Web: wire.WebRead, Handler: func(w http.ResponseWriter, _ *http.Request) {
			list, err := q().ListSessionsWithQueue()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			wire.WriteJSON(w, list)
		}},
		{Pattern: "POST /v1/sessions/create", Web: wire.WebWrite, Handler: m((*Manager).SessionCreateWrite), WebHandler: m((*Manager).SessionCreateWeb)},
		{Pattern: "POST /v1/sessions/stop", Web: wire.WebWrite, Handler: write(stop), WebHandler: agentWeb(stop)},
		{Pattern: "POST /v1/sessions/remove", Web: wire.WebWrite, Handler: write(remove), WebHandler: agentWeb(remove)},
		{Pattern: "POST /v1/sessions/mark-read", Handler: write(func() func(wire.SessionRequest) (wire.SessionResult, error) { return mg().MarkSessionRead })},
		{Pattern: "POST /v1/sessions/focus", Web: wire.WebWrite, Handler: m((*Manager).SessionFocusWrite), WebHandler: m((*Manager).SessionFocusWriteWeb)},
		{Pattern: "POST /v1/sessions/show", Handler: write(func() func(wire.SessionRequest) (wire.SessionResult, error) { return mg().FocusSession })},
		{Pattern: "POST /v1/sessions/link", Handler: res((*Resumer).SessionLinkWrite)},
		{Pattern: "POST /v1/sessions/step", Handler: res((*Resumer).SessionStepWrite)},
		{Pattern: "POST /v1/sessions/resume", Web: wire.WebWrite, Handler: write(resume), WebHandler: designWeb(resume)},
		{Pattern: "POST /v1/sessions/interrupt", Web: wire.WebWrite, Handler: write(interrupt), WebHandler: designWeb(interrupt)},
	}
}
