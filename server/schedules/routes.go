package schedules

import (
	"errors"
	"net/http"
	"strings"
	"time"

	schedrec "github.com/elentok/gx/schedules"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/wire"
)

// Schedules serves the read routes over the stored Schedules and this
// machine's state.
type Schedules struct {
	Store       *schedrec.Store
	State       *State
	TicketStore string
	// Projects lists the registered project names.
	Projects func() ([]string, error)
	// DefaultHarness is the name the Default resolves to for a project.
	DefaultHarness func(project string) string
	Now            func() time.Time // nil means time.Now
	// Events publishes schedules-changed after a write.
	Events *broker.Broker
	// Check runs one scheduler check; a delete uses it to tidy at once.
	Check func()
	// RunNow starts one run of a Schedule now.
	RunNow func(schedrec.Schedule) (wire.ScheduleRunNowResult, error)
}

// Routes lists the read routes. The getter is called per request.
func Routes(get func() *Schedules) []wire.Route {
	return append([]wire.Route{
		{Pattern: "GET /v1/schedules", Web: wire.WebRead, Handler: func(w http.ResponseWriter, r *http.Request) { get().listRead(w, r) }},
		{Pattern: "GET /v1/schedules/show", Web: wire.WebRead, Handler: func(w http.ResponseWriter, r *http.Request) { get().showRead(w, r) }},
	}, WriteRoutes(get)...)
}

func (s *Schedules) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Schedules) listRead(w http.ResponseWriter, _ *http.Request) {
	items, err := s.Store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	names, err := s.Projects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	res := wire.SchedulesResult{Schedules: []wire.ScheduleInfo{}, Projects: append([]string{}, names...)}
	now := s.now()
	for _, it := range items {
		res.Schedules = append(res.Schedules, s.info(it.Schedule, it.Registered, now))
	}
	wire.WriteJSON(w, res)
}

func (s *Schedules) showRead(w http.ResponseWriter, r *http.Request) {
	project, slug := r.URL.Query().Get("project"), r.URL.Query().Get("slug")
	sc, err := s.Store.Get(project, slug)
	switch {
	case errors.Is(err, schedrec.ErrNotFound):
		wire.WriteJSON(w, refusedShow(wire.ScheduleReasonUnknown, "no schedule "+project+":"+slug))
		return
	case schedrec.ReasonOf(err) == schedrec.ReasonInvalidProject:
		wire.WriteJSON(w, refusedShow(wire.ScheduleReasonInvalidProject, "no project "+project))
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	registered, err := s.registered(sc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	runs, err := LastRuns(s.TicketStore, s.State, Key{Project: sc.Project, Slug: sc.Slug})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if runs == nil {
		runs = []wire.ScheduleRun{}
	}
	wire.WriteJSON(w, wire.ScheduleShowResult{Schedule: s.info(sc, registered, s.now()), Prompt: sc.Prompt, LastRuns: runs})
}

func refusedShow(reason, msg string) wire.ScheduleShowResult {
	return wire.ScheduleShowResult{Refused: true, Reason: reason, Message: msg, LastRuns: []wire.ScheduleRun{}}
}

// registered reports whether sc's project directory has a project.json; Get
// does not say, so it takes the listing.
func (s *Schedules) registered(sc schedrec.Schedule) (bool, error) {
	items, err := s.Store.List()
	if err != nil {
		return false, err
	}
	for _, it := range items {
		if it.Project == sc.Project && it.Slug == sc.Slug {
			return it.Registered, nil
		}
	}
	return false, nil
}

func (s *Schedules) info(sc schedrec.Schedule, registered bool, now time.Time) wire.ScheduleInfo {
	on := s.State.Enabled(Key{Project: sc.Project, Slug: sc.Slug})
	info := wire.ScheduleInfo{
		Name: sc.Name, Slug: sc.Slug, Project: sc.Project, Times: sc.Times, Harness: sc.Harness,
		DefaultHarness: s.DefaultHarness(sc.Project), LateLimitHours: sc.LateLimitHours,
		Switch: on, ProjectFound: registered, UpdatedAt: sc.UpdatedAt.Format(time.RFC3339),
	}
	info.Days = make([]string, len(sc.Days))
	for i, d := range sc.Days {
		info.Days[i] = strings.ToLower(d.String()[:3])
	}
	if info.Times == nil {
		info.Times = []string{}
	}
	if on && registered {
		if next := schedrec.Next(sc, now.In(time.Local)); !next.IsZero() {
			info.NextRun = next.Format(time.RFC3339)
		}
	}
	return info
}
