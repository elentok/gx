package prompt

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
)

// Routes lists the prompt and key sends, the auto-advance switch and its
// cancel, and the actionable and run-action writes, which the web mux does not
// serve. The first four are web writes; the web variants need the state token of the
// session the page saw. The getter is called per request, so a route can be
// listed before its Prompt is built.
func Routes(get func() *Prompt) []wire.Route {
	return []wire.Route{
		{
			Pattern:    "POST /v1/sessions/prompt",
			Web:        wire.WebWrite,
			Handler:    func(w http.ResponseWriter, r *http.Request) { get().promptHandler(promptSourceCLI)(w, r) },
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().promptHandler(promptSourceWeb)(w, r) },
		},
		{
			Pattern:    "POST /v1/sessions/keys",
			Web:        wire.WebWrite,
			Handler:    func(w http.ResponseWriter, r *http.Request) { get().keysHandler(promptSourceCLI)(w, r) },
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().keysHandler(promptSourceWeb)(w, r) },
		},
		{
			Pattern:    "POST /v1/sessions/auto-advance",
			Web:        wire.WebWrite,
			Handler:    func(w http.ResponseWriter, r *http.Request) { get().autoAdvanceHandler(false)(w, r) },
			WebHandler: func(w http.ResponseWriter, r *http.Request) { get().autoAdvanceHandler(true)(w, r) },
		},
		{
			Pattern: "POST /v1/sessions/auto-advance/cancel",
			Web:     wire.WebWrite,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				sessions.SessionWrite(get().cancelAutoAdvanceRoute)(w, r)
			},
			WebHandler: func(w http.ResponseWriter, r *http.Request) {
				p := get()
				p.sessions.SessionWriteDesignWeb(p.cancelAutoAdvanceRoute)(w, r)
			},
		},
		{
			Pattern: "POST /v1/sessions/images",
			Web:     wire.WebWrite,
			RawBody: true,
			Handler: func(w http.ResponseWriter, r *http.Request) { get().imageUploadHandler(w, r) },
		},
		{
			Pattern: "POST /v1/sessions/actionable",
			Handler: func(w http.ResponseWriter, r *http.Request) { get().actionableWrite(w, r) },
		},
		{
			Pattern: "POST /v1/sessions/run-action",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				sessions.SessionWrite(get().RunSessionAction)(w, r)
			},
		},
	}
}

// autoAdvanceHandler serves the switch. With web set, a guard runs first and a
// refusal from it is the response.
func (p *Prompt) autoAdvanceHandler(web bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.AutoAdvanceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if web {
			res, err := p.sessions.CheckDesignWrite(wire.SessionRequest{ID: req.ID, State: req.State})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if res.Refused {
				wire.WriteJSON(w, res)
				return
			}
		}
		res, err := p.setAutoAdvance(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wire.WriteJSON(w, res)
	}
}

// setAutoAdvance flips a Design session's auto-advance switch. Turning it off
// stops a running countdown.
func (p *Prompt) setAutoAdvance(req wire.AutoAdvanceRequest) (wire.SessionResult, error) {
	dir, meta, res, err := p.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if !meta.Design {
		return sessions.Refusal(wire.ReasonSessionNotDesign, fmt.Sprintf("session %s is not a design session", req.ID)), nil
	}
	var info wire.SessionInfo
	start := false
	if err := p.sessions.UpdateSessionMeta(dir, req.ID, func(m *sessions.Meta) bool {
		m.AutoAdvanceOff = req.Off
		m.AutoSends = 0
		start = m.CanAutoAdvance() && p.autoAdvances.deadline(req.ID).IsZero()
		info = m.Info()
		return true
	}); err != nil {
		return wire.SessionResult{}, err
	}
	if req.Off {
		p.CancelAutoAdvance(req.ID, "Auto-advance switched off")
	}
	// Switching on over a raised Continue starts the countdown a run end would have.
	if start && p.StartAutoAdvance(req.ID) {
		p.events.Publish(wire.EventSessionChanged, req.ID)
	}
	return wire.SessionResult{Session: &info}, nil
}
