package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Refusal reasons of the fix-agent route, beside the session ones.
const (
	ReasonFixUnknownTicket = "unknown-ticket"
	ReasonFixNotRepair     = "not-needs-repair"
	ReasonFixInvestigating = "investigating"
)

// FixAgentRequest is the body of POST /v1/tickets/fix-agent. State is the
// token of the needs-repair row the web saw, required on the web mux.
type FixAgentRequest struct {
	Address string `json:"address"`
	State   string `json:"state,omitempty"`
}

// FixAgentResult is the session result plus whether remote control was applied.
type FixAgentResult struct {
	SessionResult `tstype:",extends"`
	RemoteControl bool `json:"remote_control"`
}

func fixAgentRefusal(reason, msg string) FixAgentResult {
	return FixAgentResult{SessionResult: sessionRefusal(reason, msg)}
}

// repairState is the web state token of a needs-repair ticket: the answerState
// of its latest round's question (zero when it has none) and its pause class.
func repairState(evs []tickets.LogEvent, class schema.PauseClass) string {
	var q tickets.LogEvent
	if rounds := tickets.Rounds(evs); len(rounds) > 0 {
		q = rounds[len(rounds)-1].Question
	}
	return answerState(q, class)
}

func (s *Server) fixAgentWrite(w http.ResponseWriter, r *http.Request) {
	s.fixAgentHandler(false)(w, r)
}

// fixAgentWeb is fixAgentWrite for the web mux: the request needs a state token.
func (s *Server) fixAgentWeb() http.HandlerFunc { return s.fixAgentHandler(true) }

func (s *Server) fixAgentHandler(web bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req FixAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := s.startFixAgent(req, web)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}

// fixAgentName is the session name of the agent fixing addr. Creating with a
// unique name makes it the one-agent-per-ticket guard.
func fixAgentName(addr string) string {
	return "fix-" + strings.NewReplacer(":", "-", "/", "-").Replace(addr)
}

// startFixAgent starts a /gx-investigate session in the project's trunk
// checkout for a needs-repair ticket.
func (s *Server) startFixAgent(req FixAgentRequest, web bool) (FixAgentResult, error) {
	if web && req.State == "" {
		return fixAgentRefusal(ReasonStale, "a state token is required; review the current pause"), nil
	}
	ref, ok, err := s.findTicket(req.Address)
	if err != nil {
		return fixAgentRefusal(ReasonSessionBadRequest, err.Error()), nil
	}
	if !ok {
		return fixAgentRefusal(ReasonFixUnknownTicket, fmt.Sprintf("no ticket %s", req.Address)), nil
	}
	addr := ref.addr.String()
	t := ref.ticket
	// The raw status, not the rendered one: an open investigate child renders
	// the parent as waiting for children, which would hide this refusal.
	for _, c := range ref.epic.Tickets {
		if c.Parent != nil && *c.Parent == t.DisplayNumber() && c.Type == string(schema.TypeInvestigate) && !c.IsTerminal() {
			return fixAgentRefusal(ReasonFixInvestigating, fmt.Sprintf("%s is already being investigated in %s", addr, ticketAddress(ref.root(), c.DisplayNumber()))), nil
		}
	}
	if t.PausedClass() != schema.PauseClassRepair {
		return fixAgentRefusal(ReasonFixNotRepair, fmt.Sprintf("%s is not needs-repair", addr)), nil
	}
	if web && req.State != repairState(readLogLenient(t), t.PausedClass()) {
		return fixAgentRefusal(ReasonStale, "the pause changed; review the current one"), nil
	}
	checkout := tickets.TrunkCheckout(s.cfg.TicketStore, ref.addr.Project)
	if checkout == "" {
		return fixAgentRefusal(ReasonSessionBadRequest, fmt.Sprintf("project %s has no trunk checkout", ref.addr.Project)), nil
	}
	prompt := "/gx-investigate " + addr + " is needs-repair"
	if label := t.PauseReason.Label(); label != "" {
		prompt += ": " + label
	}
	res, err := s.createSession(CreateSessionRequest{
		Project:    ref.addr.Project,
		Dir:        checkout,
		Name:       fixAgentName(addr),
		UniqueName: true,
		Prompt:     prompt,
		Harness:    s.settings.Load().chatHarness(),
	})
	if res.Refused && res.Reason == ReasonSessionNameTaken {
		return fixAgentRefusal(ReasonSessionNameTaken, "an agent is already fixing "+addr), nil
	}
	return FixAgentResult{SessionResult: res}, err
}
