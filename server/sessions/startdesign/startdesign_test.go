package startdesign_test

import (
	"errors"
	"testing"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/sessions/startdesign"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

type fakeHost struct {
	epics    []tickets.Epic
	trunk    string
	created  []wire.CreateSessionRequest
	taken    bool
	sessions []wire.SessionInfo
}

func (h *fakeHost) Epics(project string) ([]tickets.Epic, error) {
	if project != "proj" {
		return nil, errors.New("no project")
	}
	return h.epics, nil
}
func (h *fakeHost) ProjectDir(string) (string, error) { return "", errors.New("unused") }
func (h *fakeHost) ConversationsDir() string          { return "" }
func (h *fakeHost) LaunchHarness(string) string       { return "claude" }
func (h *fakeHost) TrunkCheckout(string) string       { return h.trunk }
func (h *fakeHost) Active(tickets.Address) error      { return nil }
func (h *fakeHost) ListSessions() ([]wire.SessionInfo, error) {
	return h.sessions, nil
}

func (h *fakeHost) CreateSession(req wire.CreateSessionRequest) (wire.SessionResult, error) {
	h.created = append(h.created, req)
	if h.taken {
		return wire.SessionResult{Refused: true, Reason: wire.ReasonSessionNameTaken}, nil
	}
	return wire.SessionResult{Session: &wire.SessionInfo{ID: "new", Name: req.Name}}, nil
}

func backlog(name string, statuses ...string) tickets.Epic {
	e := tickets.Epic{Name: name, IsBacklog: true}
	for _, s := range statuses {
		e.Tickets = append(e.Tickets, tickets.Ticket{Identifier: "01", Status: s})
	}
	return e
}

func newHost() *fakeHost {
	return &fakeHost{trunk: "/trunk", epics: []tickets.Epic{
		backlog("backlog", "open", "draft", "done"),
		backlog("empty", "done"),
		{Name: "loop", Tickets: []tickets.Ticket{{Identifier: "01", Status: "open"}}},
	}}
}

func TestStart_FlowlessWorkerSessionInTrunkWithSkillPrompt(t *testing.T) {
	t.Parallel()
	h := newHost()
	res, err := startdesign.New(h).Start(wire.StartDesignRequest{Project: "proj", Backlog: "backlog", Hint: "the  retry\nwork"})
	if err != nil || res.Refused || res.Session == nil {
		t.Fatalf("Start = %+v, %v", res, err)
	}
	got := h.created[0]
	if got.Dir != "/trunk" || got.Tier != agent.TierWorker || !got.UniqueName {
		t.Errorf("request = %+v", got)
	}
	if got.Name != "start-design-proj-backlog" {
		t.Errorf("name = %q", got.Name)
	}
	if want := "/gx-start-design proj:backlog the retry work"; got.Prompt != want {
		t.Errorf("prompt = %q, want %q", got.Prompt, want)
	}
}

func TestStart_SecondCallReturnsTheLiveChat(t *testing.T) {
	t.Parallel()
	h := newHost()
	h.taken = true
	h.sessions = []wire.SessionInfo{
		{ID: "old", Name: "start-design-proj-backlog", Status: wire.SessionStatus{State: wire.SessionExited}},
		{ID: "live", Name: "start-design-proj-backlog", Status: wire.SessionStatus{State: wire.SessionIdle}},
	}
	res, err := startdesign.New(h).Start(wire.StartDesignRequest{Project: "proj", Backlog: "backlog"})
	if err != nil || res.Refused || res.Session == nil || res.Session.ID != "live" {
		t.Fatalf("Start = %+v, %v", res, err)
	}
}

func TestStart_Refusals(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		req    wire.StartDesignRequest
		reason string
	}{
		"unknown project": {wire.StartDesignRequest{Project: "nope", Backlog: "backlog"}, wire.ReasonStartDesignUnknownProject},
		"not a backlog":     {wire.StartDesignRequest{Project: "proj", Backlog: "loop"}, wire.ReasonStartDesignNotBacklog},
		"missing epic":    {wire.StartDesignRequest{Project: "proj", Backlog: "ghost"}, wire.ReasonStartDesignNotBacklog},
		"no open ticket":  {wire.StartDesignRequest{Project: "proj", Backlog: "empty"}, wire.ReasonStartDesignNoOpenTickets},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHost()
			res, err := startdesign.New(h).Start(tc.req)
			if err != nil || !res.Refused || res.Reason != tc.reason {
				t.Errorf("Start = %+v, %v; want refusal %s", res, err, tc.reason)
			}
			if len(h.created) != 0 {
				t.Error("a refused request created a session")
			}
		})
	}
}

func TestFinishMove_RefusesNonBacklogWithoutMoving(t *testing.T) {
	t.Parallel()
	res, err := startdesign.New(newHost()).FinishMove(wire.FinishMoveRequest{Project: "proj", Backlog: "loop", Map: "m"})
	if err != nil || !res.Refused || res.Reason != wire.ReasonStartDesignNotBacklog {
		t.Errorf("FinishMove = %+v, %v", res, err)
	}
}
