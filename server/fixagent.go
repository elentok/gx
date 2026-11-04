package server

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func fixAgentRefusal(reason, msg string) wire.FixAgentResult {
	return wire.FixAgentResult{SessionResult: sessionRefusal(reason, msg)}
}

// repairState is the web state token of a ticket paused for repair: the answerState
// of its latest round's question (zero when it has none) and its pause class.
func repairState(evs []tickets.LogEvent, class schema.PauseClass) string {
	var q tickets.LogEvent
	if rounds := tickets.Rounds(evs); len(rounds) > 0 {
		q = rounds[len(rounds)-1].Question
	}
	return answerState(q, class)
}

func (rs *Resumer) fixAgentWrite(w http.ResponseWriter, r *http.Request) {
	rs.fixAgentHandler(false)(w, r)
}

// fixAgentWeb is fixAgentWrite for the web mux: the request needs a state token.
func (rs *Resumer) fixAgentWeb() http.HandlerFunc { return rs.fixAgentHandler(true) }

func (rs *Resumer) fixAgentHandler(web bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.FixAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := rs.startFixAgent(req, web)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}

// FixAgentName is the session name of the agent fixing addr. Creating with a
// unique name makes it the one-agent-per-ticket guard, so the mapping must be
// injective: "_" starts every escape, and is itself escaped, so two addresses
// never share a name even when their parts contain hyphens.
func FixAgentName(addr string) string {
	return "fix-" + strings.NewReplacer("_", "__", ":", "_c", "/", "_s").Replace(addr)
}

// fixAgentPrompt is the prompt the fix agent starts from; a pause label
// follows the colon, none leaves the sentence at the state name.
func fixAgentPrompt(agent, addr string, t tickets.Ticket) string {
	state := "paused for repair"
	switch t.RawStatus() {
	case tickets.StatusPausedAnswer:
		state = "paused for an answer"
	case tickets.StatusError:
		state = "in error"
	}
	prompt := harness.SkillCommand(agent, "gx-investigate", addr+" is "+state)
	if label := t.PauseReason.Label(); label != "" {
		prompt += ": " + label
	}
	return prompt
}

// fixProjectPrompt is the prompt of an agent fixing a project's failed migration.
func fixProjectPrompt(agent string, f wire.MigrationFailure) string {
	return harness.SkillCommand(agent, "gx-investigate", fmt.Sprintf(
		"the migration of project %s failed with: %s\nAfter fixing it, run `%s` to retry.",
		f.Project, f.Error, f.RetryCommand))
}

// startFixProjectAgent starts the agent fixing a project's saved failed migration.
func (rs *Resumer) startFixProjectAgent(project string) (wire.FixAgentResult, error) {
	var failure *wire.MigrationFailure
	for _, f := range rs.host.MigrationFailures() {
		if f.Project == project {
			failure = &f
			break
		}
	}
	if failure == nil {
		return fixAgentRefusal(wire.ReasonFixNoFailure, fmt.Sprintf("project %s has no saved migration failure", project)), nil
	}
	checkout := tickets.TrunkCheckout(rs.mg.cfg.TicketStore, project)
	if checkout == "" {
		return fixAgentRefusal(wire.ReasonSessionBadRequest, fmt.Sprintf("project %s has no trunk checkout", project)), nil
	}
	cs := wire.CreateSessionRequest{
		Project:    project,
		Dir:        checkout,
		Name:       FixAgentName(project),
		UniqueName: true,
		Prompt:     fixProjectPrompt(rs.mg.projects.LaunchHarness(project), *failure),
		Tier:       ralphloop.TierStrongest,
	}
	res, err := rs.mg.createSession(cs, sessionCtx{})
	if res.Refused && res.Reason == wire.ReasonSessionNameTaken {
		return fixAgentRefusal(wire.ReasonSessionNameTaken, "an agent is already fixing "+project), nil
	}
	return wire.FixAgentResult{SessionResult: res}, err
}

// startFixAgent starts a /gx-investigate session in the project's trunk
// checkout for a ticket paused for an answer or repair, or in error (web: repair only),
// or for a project's failed migration.
func (rs *Resumer) startFixAgent(req wire.FixAgentRequest, web bool) (wire.FixAgentResult, error) {
	if req.Project != "" {
		return rs.startFixProjectAgent(req.Project)
	}
	if web && req.State == "" {
		return fixAgentRefusal(wire.ReasonStale, "a state token is required; review the current pause"), nil
	}
	ref, ok, err := rs.host.findTicket(req.Address)
	if err != nil {
		return fixAgentRefusal(wire.ReasonSessionBadRequest, err.Error()), nil
	}
	if !ok {
		return fixAgentRefusal(wire.ReasonFixUnknownTicket, fmt.Sprintf("no ticket %s", req.Address)), nil
	}
	addr := ref.addr.String()
	t := ref.ticket
	// The raw status, not the rendered one: an open investigate child renders
	// the parent as waiting for children, which would hide this refusal.
	for _, c := range ref.epic.Tickets {
		if c.Parent != nil && *c.Parent == t.DisplayNumber() && c.Type == string(schema.TypeInvestigate) && !c.IsTerminal() {
			return fixAgentRefusal(wire.ReasonFixInvestigating, fmt.Sprintf("%s is already being investigated in %s", addr, ticketAddress(ref.root(), c.DisplayNumber()))), nil
		}
	}
	// The web button stays repair-only; the TUI's Investigate also takes an
	// answer-paused or error ticket.
	if web {
		if t.PausedClass() != schema.PauseClassRepair {
			return fixAgentRefusal(wire.ReasonFixNotRepair, fmt.Sprintf("%s is not paused for repair", addr)), nil
		}
	} else if st := ref.epic.RenderedStatus(t); st != tickets.StatusPausedAnswer && st != tickets.StatusPausedRepair && st != tickets.StatusError {
		return fixAgentRefusal(wire.ReasonFixNotRepair, fmt.Sprintf("%s is not paused or in error", addr)), nil
	}
	if web && req.State != repairState(readLogLenient(t), t.PausedClass()) {
		return fixAgentRefusal(wire.ReasonStale, "the pause changed; review the current one"), nil
	}
	checkout := tickets.TrunkCheckout(rs.mg.cfg.TicketStore, ref.addr.Project)
	if checkout == "" {
		return fixAgentRefusal(wire.ReasonSessionBadRequest, fmt.Sprintf("project %s has no trunk checkout", ref.addr.Project)), nil
	}
	settings := rs.mg.settings.Load()
	cs := wire.CreateSessionRequest{
		Project:    ref.addr.Project,
		Dir:        checkout,
		Name:       FixAgentName(addr),
		UniqueName: true,
		Prompt:     fixAgentPrompt(rs.mg.projects.LaunchHarness(ref.addr.Project), addr, t),
		Tier:       ralphloop.TierStrongest,
	}
	remote := settings.AgentsFixAgentRemoteControl && rs.mg.projects.LaunchHarness(cs.Project) == cmp.Or(settings.RemoteControlHarness, string(harness.Claude))
	var sc sessionCtx
	if remote {
		sc.remoteControl = addr
	}
	res, err := rs.mg.createSession(cs, sc)
	if res.Refused && res.Reason == wire.ReasonSessionNameTaken {
		return fixAgentRefusal(wire.ReasonSessionNameTaken, "an agent is already fixing "+addr), nil
	}
	return wire.FixAgentResult{SessionResult: res, RemoteControl: remote && !res.Refused && err == nil}, err
}
