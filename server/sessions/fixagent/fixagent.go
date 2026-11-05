// Package fixagent starts the /gx-investigate session that fixes a paused
// ticket or a project's failed migration. It sits above server/sessions: the
// root hands it a Host that creates the session.
package fixagent

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketgit"
)

// Host is what the fix agent needs from the rest of the server.
type Host interface {
	FindTicket(address string) (queue.TicketRef, bool, error)
	MigrationFailures() []wire.MigrationFailure
	// CreateSession creates the session; remoteControl, when set, is the
	// --remote-control name it starts under.
	CreateSession(req wire.CreateSessionRequest, remoteControl string) (wire.SessionResult, error)
	LaunchHarness(project string) string
	TicketStore() string
	Settings() *settings.Settings
}

// FixAgent starts fix agents.
type FixAgent struct{ host Host }

// New builds the FixAgent over host.
func New(host Host) *FixAgent { return &FixAgent{host: host} }

func refusal(reason, msg string) wire.FixAgentResult {
	return wire.FixAgentResult{SessionResult: wire.SessionResult{Refused: true, Reason: reason, Message: msg}}
}

// RepairState is the web state token of a ticket paused for repair: the answerState
// of its latest round's question (zero when it has none) and its pause class.
func RepairState(evs []tickets.LogEvent, class schema.PauseClass) string {
	var q tickets.LogEvent
	if rounds := tickets.Rounds(evs); len(rounds) > 0 {
		q = rounds[len(rounds)-1].Question
	}
	return queue.AnswerState(q, class)
}

// Write is the socket handler.
func (fa *FixAgent) Write(w http.ResponseWriter, r *http.Request) {
	fa.handler(false)(w, r)
}

// Web is Write for the web mux: the request needs a state token.
func (fa *FixAgent) Web() http.HandlerFunc { return fa.handler(true) }

func (fa *FixAgent) handler(web bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.FixAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := fa.start(req, web)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wire.WriteJSON(w, res)
	}
}

// FixAgentName is the session name of the agent fixing addr. Creating with a
// unique name makes it the one-agent-per-ticket guard, so the mapping must be
// injective: "_" starts every escape, and is itself escaped, so two addresses
// never share a name even when their parts contain hyphens.
func FixAgentName(addr string) string {
	return "fix-" + strings.NewReplacer("_", "__", ":", "_c", "/", "_s").Replace(addr)
}

// FixAgentPrompt is the prompt the fix agent starts from; a pause label
// follows the colon, none leaves the sentence at the state name.
func FixAgentPrompt(agentKind, addr string, t tickets.Ticket) string {
	state := "paused for repair"
	switch t.RawStatus() {
	case tickets.StatusPausedAnswer:
		state = "paused for an answer"
	case tickets.StatusError:
		state = "in error"
	}
	prompt := harness.SkillCommand(agentKind, "gx-investigate", addr+" is "+state)
	if label := t.PauseReason.Label(); label != "" {
		prompt += ": " + label
	}
	return prompt
}

// fixProjectPrompt is the prompt of an agent fixing a project's failed migration.
func fixProjectPrompt(agentKind string, f wire.MigrationFailure) string {
	return harness.SkillCommand(agentKind, "gx-investigate", fmt.Sprintf(
		"the migration of project %s failed with: %s\nAfter fixing it, run `%s` to retry.",
		f.Project, f.Error, f.RetryCommand))
}

// startProject starts the agent fixing a project's saved failed migration.
func (fa *FixAgent) startProject(project string) (wire.FixAgentResult, error) {
	var failure *wire.MigrationFailure
	for _, f := range fa.host.MigrationFailures() {
		if f.Project == project {
			failure = &f
			break
		}
	}
	if failure == nil {
		return refusal(wire.ReasonFixNoFailure, fmt.Sprintf("project %s has no saved migration failure", project)), nil
	}
	checkout := ticketgit.TrunkCheckout(fa.host.TicketStore(), project)
	if checkout == "" {
		return refusal(wire.ReasonSessionBadRequest, fmt.Sprintf("project %s has no trunk checkout", project)), nil
	}
	cs := wire.CreateSessionRequest{
		Project:    project,
		Dir:        checkout,
		Name:       FixAgentName(project),
		UniqueName: true,
		Prompt:     fixProjectPrompt(fa.host.LaunchHarness(project), *failure),
		Tier:       agent.TierStrongest,
	}
	res, err := fa.host.CreateSession(cs, "")
	if res.Refused && res.Reason == wire.ReasonSessionNameTaken {
		return refusal(wire.ReasonSessionNameTaken, "an agent is already fixing "+project), nil
	}
	return wire.FixAgentResult{SessionResult: res}, err
}

// start starts a /gx-investigate session in the project's trunk
// checkout for a ticket paused for an answer or repair, or in error (web: repair only),
// or for a project's failed migration.
func (fa *FixAgent) start(req wire.FixAgentRequest, web bool) (wire.FixAgentResult, error) {
	if req.Project != "" {
		return fa.startProject(req.Project)
	}
	if web && req.State == "" {
		return refusal(wire.ReasonStale, "a state token is required; review the current pause"), nil
	}
	ref, ok, err := fa.host.FindTicket(req.Address)
	if err != nil {
		return refusal(wire.ReasonSessionBadRequest, err.Error()), nil
	}
	if !ok {
		return refusal(wire.ReasonFixUnknownTicket, fmt.Sprintf("no ticket %s", req.Address)), nil
	}
	addr := ref.Addr.String()
	t := ref.Ticket
	// The raw status, not the rendered one: an open investigate child renders
	// the parent as waiting for children, which would hide this refusal.
	for _, c := range ref.Epic.Tickets {
		if c.Parent != nil && *c.Parent == t.DisplayNumber() && c.Type == string(schema.TypeInvestigate) && !c.IsTerminal() {
			return refusal(wire.ReasonFixInvestigating, fmt.Sprintf("%s is already being investigated in %s", addr, ref.Root().TicketAddress(c.DisplayNumber()))), nil
		}
	}
	// The web button stays repair-only; the TUI's Investigate also takes an
	// answer-paused or error ticket.
	if web {
		if t.PausedClass() != schema.PauseClassRepair {
			return refusal(wire.ReasonFixNotRepair, fmt.Sprintf("%s is not paused for repair", addr)), nil
		}
	} else if st := ref.Epic.RenderedStatus(t); st != tickets.StatusPausedAnswer && st != tickets.StatusPausedRepair && st != tickets.StatusError {
		return refusal(wire.ReasonFixNotRepair, fmt.Sprintf("%s is not paused or in error", addr)), nil
	}
	if web && req.State != RepairState(queue.ReadLogLenient(t), t.PausedClass()) {
		return refusal(wire.ReasonStale, "the pause changed; review the current one"), nil
	}
	checkout := ticketgit.TrunkCheckout(fa.host.TicketStore(), ref.Addr.Project)
	if checkout == "" {
		return refusal(wire.ReasonSessionBadRequest, fmt.Sprintf("project %s has no trunk checkout", ref.Addr.Project)), nil
	}
	settings := fa.host.Settings()
	agentKind := fa.host.LaunchHarness(ref.Addr.Project)
	cs := wire.CreateSessionRequest{
		Project:    ref.Addr.Project,
		Dir:        checkout,
		Name:       FixAgentName(addr),
		UniqueName: true,
		Prompt:     FixAgentPrompt(agentKind, addr, t),
		Tier:       agent.TierStrongest,
	}
	remote := settings.AgentsFixAgentRemoteControl && agentKind == cmp.Or(settings.RemoteControlHarness, string(harness.Claude))
	remoteControl := ""
	if remote {
		remoteControl = addr
	}
	res, err := fa.host.CreateSession(cs, remoteControl)
	if res.Refused && res.Reason == wire.ReasonSessionNameTaken {
		return refusal(wire.ReasonSessionNameTaken, "an agent is already fixing "+addr), nil
	}
	return wire.FixAgentResult{SessionResult: res, RemoteControl: remote && !res.Refused && err == nil}, err
}

// Routes lists the fix-agent endpoint. fa is called per request, so the route
// can be listed before the FixAgent is built.
func Routes(fa func() *FixAgent) []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/tickets/fix-agent", Web: wire.WebWrite,
			Handler:    func(w http.ResponseWriter, r *http.Request) { fa().Write(w, r) },
			WebHandler: func(w http.ResponseWriter, r *http.Request) { fa().Web()(w, r) }},
	}
}
