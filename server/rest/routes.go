package rest

import (
	"encoding/json"
	"net/http"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
)

// Status is the current status. It changes nothing.
func (r *Runner) Status() wire.RestStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	cfg := r.cfg()
	res := Step(Input{State: r.state, Config: cfg, Now: r.now(), Event: Event{Kind: EventTick}})
	return statusOf(cfg, res)
}

func statusOf(cfg config.MentalHealthConfig, res Result) wire.RestStatus {
	v := res.View
	_, offset := v.Now.Zone()
	st := wire.RestStatus{
		Enabled:           cfg.Enabled,
		Counting:          v.Counting,
		WorkCount:         res.State.WorkCount,
		Now:               v.Now,
		ZoneOffsetSeconds: offset,
		Rule: wire.RestRule{
			WorkMinutes: cfg.Break.WorkMinutes, BreakMinutes: cfg.Break.BreakMinutes,
			BedtimeStart: cfg.Bedtime.Start, BedtimeEnd: cfg.Bedtime.End,
		},
	}
	// A clock without a name ("Local") cannot be rebuilt by the web, so only
	// the offset travels.
	if name := v.Now.Location().String(); name != "Local" {
		st.Zone = name
	}
	if p := v.Period; p != nil {
		st.Period = toWire(*p)
	}
	if w := v.Warning; w != nil {
		st.Warning = &wire.RestWarning{Kind: string(w.Kind), Start: w.Start, BreakMinutes: w.BreakMinutes}
	}
	return st
}

func toWire(p Period) *wire.RestPeriod {
	return &wire.RestPeriod{Kind: string(p.Kind), Start: p.Start, End: p.End, Message: p.Message, SubMessage: p.SubMessage}
}

// refusalMessages are the sentences the CLI prints for a refusal reason.
var refusalMessages = map[string]string{
	wire.ReasonRestSentenceMismatch: `type "I'm <your name> and I'm addicted to AI" to unlock`,
	wire.ReasonRestNoPeriod:         "no Rest period is running",
}

func result(cfg config.MentalHealthConfig, res Result) wire.RestResult {
	out := wire.RestResult{Status: statusOf(cfg, res)}
	if res.Refusal != "" {
		out.Refused, out.Reason, out.Message = true, res.Refusal, refusalMessages[res.Refusal]
	}
	return out
}

// Routes lists the Rest endpoints. The getter is called per request, so a
// route can be listed before its Runner is built.
func Routes(runner func() *Runner) []wire.Route {
	write := func(pattern string, do func(*Runner, *http.Request) (Result, error)) wire.Route {
		return wire.Route{Pattern: pattern, Web: wire.WebWrite, Handler: func(w http.ResponseWriter, req *http.Request) {
			rn := runner()
			res, err := do(rn, req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			wire.WriteJSON(w, result(rn.cfg(), res))
		}}
	}
	return []wire.Route{
		{Pattern: "GET /v1/rest", Handler: func(w http.ResponseWriter, _ *http.Request) { wire.WriteJSON(w, runner().Status()) }},
		write("POST /v1/rest/active", func(rn *Runner, _ *http.Request) (Result, error) { return rn.Active() }),
		{Pattern: "POST /v1/rest/unlock", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, req *http.Request) {
			var body wire.RestUnlockRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			rn := runner()
			res, err := rn.Unlock(body.Sentence)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			wire.WriteJSON(w, result(rn.cfg(), res))
		}},
	}
}
