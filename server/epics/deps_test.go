package epics

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// The fakes below stand in for the epics.Deps interfaces, so the read models run
// without a Server behind them.

type fakeSessions struct {
	list []wire.SessionInfo
	err  error
}

func (f fakeSessions) ListSessions() ([]wire.SessionInfo, error) { return f.list, f.err }

type fakeQueue struct{ items []wire.QueueItem }

func (f fakeQueue) List() []wire.QueueItem      { return f.items }
func (fakeQueue) RootQueued(queue.RootRef) bool { return false }

type fakeHistory struct{ err error }

func (f fakeHistory) ReadHistory(tickets.Address) ([]runlog.Event, error) { return nil, f.err }

type fakeWriter struct{}

func (fakeWriter) FindTicket(string) (queue.TicketRef, bool, error) {
	return queue.TicketRef{}, false, nil
}

type fakeSettings struct{ recentlyFinished int }

func (f fakeSettings) Load() *settings.Settings {
	return &settings.Settings{RecentlyFinished: f.recentlyFinished}
}

type fakeHarness struct {
	label wire.HarnessLabel
	live  bool
}

func (f fakeHarness) LiveSession(tickets.Address) (agentrunner.Runner, agentrunner.Session, bool, error) {
	return nil, agentrunner.Session{}, f.live, nil
}

func (f fakeHarness) LabelFor(tickets.Address, string, string) wire.HarnessLabel { return f.label }

type fakeProjects struct {
	epics map[string][]tickets.Epic
	asked []string
}

func (f *fakeProjects) Epics(project string) ([]tickets.Epic, error) {
	f.asked = append(f.asked, project)
	return f.epics[project], nil
}

func (*fakeProjects) LaunchHarness(string) string { return "claude" }

type fakeRuns struct{}

func (fakeRuns) StartedAtOf(string) (time.Time, bool) { return time.Time{}, false }

func (fakeRuns) LaunchStepOf(string) (string, int) { return "", 0 }

type fakePauses struct{}

func (fakePauses) State(string) string { return "" }

type fakeChat struct{}

func (fakeChat) TicketPtyDir(tickets.Address) (string, bool) { return "", false }

func fakeEpics() Deps {
	return Deps{
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Harness:     fakeHarness{},
		Projects:    &fakeProjects{},
		Chat:        fakeChat{},
		Settings:    fakeSettings{},
		Writer:      fakeWriter{},
		Runs:        fakeRuns{},
		History:     fakeHistory{},
		Queue:       fakeQueue{},
		Pauses:      fakePauses{},
		Sessions:    fakeSessions{},
		AutoFFMerge: func(string) bool { return true },
	}
}

func TestEpicsDeps_DesignActivitiesReadsTheSessionsFake(t *testing.T) {
	d := fakeEpics()
	d.Sessions = fakeSessions{list: []wire.SessionInfo{{Design: true, Ticket: "p:map/01"}}}
	s := New(d)
	got := s.designActivities()
	if a := got[queue.EpicAddress("p:map/01")]; !a.running {
		t.Fatalf("activities = %+v, want the map running", got)
	}
}

func TestEpicsDeps_DesignActivitiesIsEmptyWhenSessionsFail(t *testing.T) {
	d := fakeEpics()
	d.Sessions = fakeSessions{err: errors.New("boom")}
	s := New(d)
	if got := s.designActivities(); len(got) != 0 {
		t.Fatalf("activities = %+v, want none", got)
	}
}

func TestEpicsDeps_QueueIndexReadsTheQueueFake(t *testing.T) {
	d := fakeEpics()
	d.Queue = fakeQueue{items: []wire.QueueItem{{Address: "p:e/01"}, {Address: "p:f/01"}}}
	s := New(d)
	q := s.newQueueIndex()
	if got := q.slot(queueKey(tickets.Address{Project: "p", Epic: "f", ID: "1"})); got != 2 {
		t.Fatalf("slot of the second queued epic = %d, want 2", got)
	}
}

func TestEpicsDeps_TicketRunsReturnsTheHistoryError(t *testing.T) {
	d := fakeEpics()
	want := errors.New("history unreadable")
	d.History = fakeHistory{err: want}
	s := New(d)
	if _, err := s.ticketRuns(queue.TicketRef{}, schema.Ticket{}, nil, time.Time{}); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestEpicsDeps_DetailAnswers404WhenTheWriterFindsNothing(t *testing.T) {
	s := New(fakeEpics())
	rec := httptest.NewRecorder()
	s.Detail(rec, httptest.NewRequest(http.MethodGet, "/v1/detail?address=p:e/01", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestEpicsDeps_PermissionCardIsNilWithoutALiveSession(t *testing.T) {
	s := New(fakeEpics())
	card, err := s.PermissionCard(tickets.Address{Project: "p", Epic: "e", ID: "1"})
	if card != nil || err != nil {
		t.Fatalf("card, err = %v, %v, want nil, nil", card, err)
	}
	if s.needsApproval("p:e/01") {
		t.Fatal("needsApproval = true with no live session")
	}
}

func TestEpicsDeps_RowForUsesTheHarnessFake(t *testing.T) {
	d := fakeEpics()
	d.Harness = fakeHarness{label: wire.HarnessLabel{Harness: "codex"}}
	s := New(d)
	e := tickets.Epic{Name: "e"}
	row := s.rowFor(queue.RootRef{Project: "p", Epic: "e"}, e, tickets.Ticket{Number: 1, Identifier: "t1"})
	if row.HarnessLabel.Harness != "codex" {
		t.Fatalf("harness label = %+v, want codex", row.HarnessLabel)
	}
}

func TestEpicsDeps_EpicSummariesAskTheProjectsFake(t *testing.T) {
	d := fakeEpics()
	proj := &fakeProjects{epics: map[string][]tickets.Epic{"p": {{Name: "e"}}}}
	d.Projects = proj
	d.Queue = fakeQueue{items: []wire.QueueItem{{Address: "p:e/01"}}}
	s := New(d)
	out := s.EpicSummaries(nil)
	if len(out) != 1 || len(proj.asked) != 1 || proj.asked[0] != "p" {
		t.Fatalf("summaries = %d, asked = %v, want one summary from project p", len(out), proj.asked)
	}
}

func TestEpicsDeps_EpicListReadsTheSettingsFake(t *testing.T) {
	d := fakeEpics()
	d.TicketStore = t.TempDir()
	d.Settings = fakeSettings{recentlyFinished: 7}
	s := New(d)
	rec := httptest.NewRecorder()
	s.EpicList(rec, httptest.NewRequest(http.MethodGet, "/v1/epics", nil))
	var list wire.EpicList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.RecentlyFinished != 7 {
		t.Fatalf("RecentlyFinished = %d, want 7", list.RecentlyFinished)
	}
}
