// Package startdesign serves the Backlog's Start Design: a plain chat that
// proposes a Design theme for a backlog, and the "finish the move" that carries
// on an unfinished move into a charting map. It sits above server/sessions:
// the root hands it a Host that creates the session.
package startdesign

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketmove"
)

// Skill is the manual skill the chat starts from.
const Skill = "gx-start-design"

// Host is what Start Design needs from the rest of the server.
type Host interface {
	// Epics is the project's epics; an error means the project is unknown.
	Epics(project string) ([]tickets.Epic, error)
	// ProjectDir is the project's ticket directory.
	ProjectDir(project string) (string, error)
	ConversationsDir() string
	CreateSession(req wire.CreateSessionRequest) (wire.SessionResult, error)
	ListSessions() ([]wire.SessionInfo, error)
	LaunchHarness(project string) string
	// TrunkCheckout is the project's trunk checkout; empty when it has none.
	TrunkCheckout(project string) string
	// Active refuses a ticket that is queued or live.
	Active(a tickets.Address) error
}

// StartDesign starts the chats and finishes the moves.
type StartDesign struct{ host Host }

// New builds StartDesign over host.
func New(host Host) *StartDesign { return &StartDesign{host: host} }

func refusal(reason, msg string) wire.SessionResult {
	return wire.SessionResult{Refused: true, Reason: reason, Message: msg}
}

// SessionName is the fixed name of the chat for a backlog. It is the
// one-chat-per-backlog guard, as the Fix agent's name is for a ticket.
func SessionName(project, backlog string) string { return "start-design-" + project + "-" + backlog }

// Prompt is the chat's first prompt: the backlog address, then the hint on one line.
func Prompt(harnessName, project, backlog, hint string) string {
	arg := project + ":" + backlog
	if h := strings.Join(strings.Fields(hint), " "); h != "" {
		arg += " " + h
	}
	return harness.SkillCommand(harnessName, Skill, arg)
}

// backlog finds the backlog epic of project, or the refusal.
func (s *StartDesign) backlog(project, name string) (tickets.Epic, *wire.SessionResult) {
	epics, err := s.host.Epics(project)
	if err != nil {
		r := refusal(wire.ReasonStartDesignUnknownProject, fmt.Sprintf("no project %s", project))
		return tickets.Epic{}, &r
	}
	for _, e := range epics {
		if e.Name == name && e.IsBacklog {
			return e, nil
		}
	}
	r := refusal(wire.ReasonStartDesignNotBacklog, fmt.Sprintf("%s is not a backlog of %s", name, project))
	return tickets.Epic{}, &r
}

// Start starts the backlog's chat, or returns the live one.
func (s *StartDesign) Start(req wire.StartDesignRequest) (wire.SessionResult, error) {
	e, refused := s.backlog(req.Project, req.Backlog)
	if refused != nil {
		return *refused, nil
	}
	if e.OpenTicketCount() == 0 {
		return refusal(wire.ReasonStartDesignNoOpenTickets, fmt.Sprintf("%s has no open ticket", req.Backlog)), nil
	}
	checkout := s.host.TrunkCheckout(req.Project)
	if checkout == "" {
		return refusal(wire.ReasonSessionBadRequest, fmt.Sprintf("project %s has no trunk checkout", req.Project)), nil
	}
	name := SessionName(req.Project, req.Backlog)
	res, err := s.host.CreateSession(wire.CreateSessionRequest{
		Project:    req.Project,
		Dir:        checkout,
		Name:       name,
		UniqueName: true,
		Prompt:     Prompt(s.host.LaunchHarness(req.Project), req.Project, req.Backlog, req.Hint),
		Tier:       agent.TierWorker,
	})
	if err != nil || !res.Refused || res.Reason != wire.ReasonSessionNameTaken {
		return res, err
	}
	// The second click opens the first chat.
	list, err := s.host.ListSessions()
	if err != nil {
		return res, err
	}
	for i := range list {
		if list[i].Name == name && list[i].Status.State != wire.SessionExited {
			return wire.SessionResult{Session: &list[i]}, nil
		}
	}
	return res, nil
}

// FinishMove carries on an unfinished move into the map, with no ticket list:
// the map's Notes name them. It starts no chat.
func (s *StartDesign) FinishMove(req wire.FinishMoveRequest) (wire.FinishMoveResult, error) {
	fail := func(reason, msg string) (wire.FinishMoveResult, error) {
		return wire.FinishMoveResult{Refused: true, Reason: reason, Message: msg}, nil
	}
	if _, refused := s.backlog(req.Project, req.Backlog); refused != nil {
		return fail(refused.Reason, refused.Message)
	}
	dir, err := s.host.ProjectDir(req.Project)
	if err != nil {
		return fail(wire.ReasonStartDesignUnknownProject, err.Error())
	}
	name, err := ticketmove.StartDesign(ticketmove.StartDesignOpts{
		ConversationsDir: s.host.ConversationsDir(), ProjectDir: dir, Project: req.Project,
		Backlog: req.Backlog, Map: req.Map, Title: req.Map, Active: s.host.Active,
	})
	if err != nil {
		return fail(wire.ReasonStartDesignMoveFailed, err.Error())
	}
	return wire.FinishMoveResult{Address: req.Project + ":" + name}, nil
}

func decode[Req, Res any](call func(Req) (Res, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Req
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := call(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wire.WriteJSON(w, res)
	}
}

// Routes lists the endpoints. sd is called per request, so the routes can be
// listed before StartDesign is built.
func Routes(sd func() *StartDesign) []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/backlog/start-design", Web: wire.WebWrite,
			Handler: decode(func(r wire.StartDesignRequest) (wire.SessionResult, error) { return sd().Start(r) })},
		{Pattern: "POST /v1/backlog/finish-move", Web: wire.WebWrite,
			Handler: decode(func(r wire.FinishMoveRequest) (wire.FinishMoveResult, error) { return sd().FinishMove(r) })},
	}
}
