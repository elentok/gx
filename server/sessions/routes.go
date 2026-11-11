package sessions

import (
	"encoding/json"
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
	// targets serves stop and remove, which take many sessions.
	targets := func(do func(*Manager) func(string, bool) (wire.SessionResult, error), web bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { mg().SessionTargetsWrite(do(mg()), web)(w, r) }
	}
	stopTarget := func(m *Manager) func(string, bool) (wire.SessionResult, error) {
		return func(id string, _ bool) (wire.SessionResult, error) { return m.StopSession(wire.SessionRequest{ID: id}) }
	}
	removeTarget := func(m *Manager) func(string, bool) (wire.SessionResult, error) { return m.RemoveTarget }
	designWeb := func(do func() func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { mg().SessionWriteDesignWeb(do())(w, r) }
	}
	interruptWeb := func(do func() func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { mg().SessionWriteInterruptWeb(do())(w, r) }
	}
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
		{Pattern: "POST /v1/sessions/stop", Web: wire.WebWrite, Handler: targets(stopTarget, false), WebHandler: targets(stopTarget, true)},
		{Pattern: "POST /v1/sessions/remove", Web: wire.WebWrite, Handler: targets(removeTarget, false), WebHandler: targets(removeTarget, true)},
		{Pattern: "POST /v1/sessions/remove-exited", Handler: func(w http.ResponseWriter, r *http.Request) {
			var req wire.RemoveExitedRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			res, err := mg().RemoveExitedSessions(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			wire.WriteJSON(w, res)
		}},
		{Pattern: "POST /v1/sessions/mark-read", Handler: write(func() func(wire.SessionRequest) (wire.SessionResult, error) { return mg().MarkSessionRead })},
		{Pattern: "POST /v1/sessions/focus", Web: wire.WebWrite, Handler: m((*Manager).SessionFocusWrite), WebHandler: m((*Manager).SessionFocusWriteWeb)},
		{Pattern: "POST /v1/sessions/show", Handler: write(func() func(wire.SessionRequest) (wire.SessionResult, error) { return mg().FocusSession })},
		{Pattern: "POST /v1/sessions/link", Handler: res((*Resumer).SessionLinkWrite)},
		{Pattern: "POST /v1/sessions/step", Handler: res((*Resumer).SessionStepWrite)},
		{Pattern: "POST /v1/sessions/resume", Web: wire.WebWrite, Handler: write(resume), WebHandler: designWeb(resume)},
		{Pattern: "POST /v1/sessions/interrupt", Web: wire.WebWrite, Handler: write(interrupt), WebHandler: interruptWeb(interrupt)},
	}
}
