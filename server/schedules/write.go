package schedules

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	schedrec "github.com/elentok/gx/schedules"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/wire"
)

var dayNames = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// WriteRoutes lists the write routes. The getter is called per request.
func WriteRoutes(get func() *Schedules) []wire.Route {
	return []wire.Route{
		{Pattern: "POST /v1/schedules/create", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { writeHandler(w, r, get().create) }},
		{Pattern: "POST /v1/schedules/edit", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { writeHandler(w, r, get().edit) }},
		{Pattern: "POST /v1/schedules/switch", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { writeHandler(w, r, get().switchTo) }},
		{Pattern: "POST /v1/schedules/delete", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { writeHandler(w, r, get().remove) }},
		{Pattern: "POST /v1/schedules/run-now", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) { writeHandler(w, r, get().runNow) }},
	}
}

func writeHandler[Req, Res any](w http.ResponseWriter, r *http.Request, fn func(Req) (Res, error)) {
	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := fn(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

func parseDays(names []string) ([]time.Weekday, error) {
	days := make([]time.Weekday, 0, len(names))
	for _, n := range names {
		i := slices.Index(dayNames, strings.ToLower(strings.TrimSpace(n)))
		if i < 0 {
			return nil, errors.New("invalid day " + n)
		}
		days = append(days, time.Weekday(i))
	}
	return days, nil
}

func (s *Schedules) registeredProject(project string) (bool, error) {
	if project == projects.ScratchProject {
		return true, nil
	}
	names, err := s.Projects()
	return slices.Contains(names, project), err
}

// lateLimit applies a requested Late limit; a refusal result is returned when
// it is not above 0 (Validate would silently default a 0).
func lateLimit(sc *schedrec.Schedule, hours *float64) (wire.ScheduleShowResult, bool) {
	if hours == nil {
		return wire.ScheduleShowResult{}, false
	}
	if !(*hours > 0) {
		return refusedShow(schedrec.ReasonInvalidLateLimit, "late limit must be above 0"), true
	}
	sc.LateLimitHours = *hours
	return wire.ScheduleShowResult{}, false
}

func (s *Schedules) create(req wire.ScheduleCreateRequest) (wire.ScheduleShowResult, error) {
	project := req.Project
	if project == "" {
		project = projects.ScratchProject
	}
	ok, err := s.registeredProject(project)
	if err != nil {
		return wire.ScheduleShowResult{}, err
	}
	if !ok {
		return refusedShow(schedrec.ReasonInvalidProject, "no project "+project), nil
	}
	days, err := parseDays(req.Days)
	if err != nil {
		return refusedShow(schedrec.ReasonNoDays, err.Error()), nil
	}
	sc := schedrec.Schedule{Name: req.Name, Prompt: req.Prompt, Days: days, Times: req.Times, Harness: req.Harness}
	if res, refused := lateLimit(&sc, req.LateLimitHours); refused {
		return res, nil
	}
	created, err := s.Store.Create(project, sc)
	if res, handled := refusalOf(err); handled {
		return res, nil
	} else if err != nil {
		return wire.ScheduleShowResult{}, err
	}
	// A new Schedule is on here only; other machines show it "Off here".
	if err := s.State.Enable(Key{Project: created.Project, Slug: created.Slug}, created.CreatedAt); err != nil {
		return wire.ScheduleShowResult{}, err
	}
	return s.written(created)
}

func (s *Schedules) edit(req wire.ScheduleEditRequest) (wire.ScheduleShowResult, error) {
	if req.NewProject != nil && *req.NewProject != "" && *req.NewProject != req.Project {
		return refusedShow(wire.ScheduleReasonProjectReadOnly, "the project of a Schedule cannot change"), nil
	}
	sc, err := s.Store.Get(req.Project, req.Slug)
	switch {
	case errors.Is(err, schedrec.ErrNotFound):
		return refusedShow(wire.ScheduleReasonUnknown, "no schedule "+req.Project+":"+req.Slug), nil
	case err != nil:
		if res, handled := refusalOf(err); handled {
			return res, nil
		}
		return wire.ScheduleShowResult{}, err
	}
	if req.Name != nil {
		sc.Name = *req.Name
	}
	if req.Prompt != nil {
		sc.Prompt = *req.Prompt
	}
	if req.Days != nil {
		days, err := parseDays(req.Days)
		if err != nil {
			return refusedShow(schedrec.ReasonNoDays, err.Error()), nil
		}
		sc.Days = days
	}
	if req.Times != nil {
		sc.Times = req.Times
	}
	if req.Harness != nil {
		sc.Harness = *req.Harness
	}
	if res, refused := lateLimit(&sc, req.LateLimitHours); refused {
		return res, nil
	}
	updated, err := s.Store.Update(req.Project, req.Slug, sc)
	if res, handled := refusalOf(err); handled {
		return res, nil
	} else if err != nil {
		return wire.ScheduleShowResult{}, err
	}
	// The edit applies at Save: no earlier time runs on this machine.
	if err := s.State.Advance(Key{Project: updated.Project, Slug: updated.Slug}, updated.UpdatedAt); err != nil {
		return wire.ScheduleShowResult{}, err
	}
	return s.written(updated)
}

// switchTo writes only this machine's state, never the Schedule file. Turning
// on sets the baseline to now, so time spent off never runs; turning off
// leaves a queued run alone.
func (s *Schedules) switchTo(req wire.ScheduleSwitchRequest) (wire.ScheduleShowResult, error) {
	sc, err := s.Store.Get(req.Project, req.Slug)
	switch {
	case errors.Is(err, schedrec.ErrNotFound):
		return refusedShow(wire.ScheduleReasonUnknown, "no schedule "+req.Project+":"+req.Slug), nil
	case err != nil:
		if res, handled := refusalOf(err); handled {
			return res, nil
		}
		return wire.ScheduleShowResult{}, err
	}
	k := Key{Project: sc.Project, Slug: sc.Slug}
	if req.On {
		err = s.State.Enable(k, s.now())
	} else {
		err = s.State.Disable(k)
	}
	if err != nil {
		return wire.ScheduleShowResult{}, err
	}
	return s.written(sc)
}

// remove deletes the Schedule file, then runs one check so this machine's own
// queued runs are cancelled and its state dropped without waiting for a tick.
func (s *Schedules) remove(req wire.ScheduleDeleteRequest) (wire.ScheduleDeleteResult, error) {
	err := s.Store.Delete(req.Project, req.Slug)
	switch {
	case errors.Is(err, schedrec.ErrNotFound):
		return wire.ScheduleDeleteResult{Refused: true, Reason: wire.ScheduleReasonUnknown, Message: "no schedule " + req.Project + ":" + req.Slug}, nil
	case err != nil:
		if reason := schedrec.ReasonOf(err); reason != "" {
			return wire.ScheduleDeleteResult{Refused: true, Reason: reason, Message: err.Error()}, nil
		}
		return wire.ScheduleDeleteResult{}, err
	}
	if s.Check != nil {
		s.Check()
	}
	s.publish(req.Project, req.Slug)
	return wire.ScheduleDeleteResult{}, nil
}

// runNow starts one run of the Schedule at once, whatever the switch says.
func (s *Schedules) runNow(req wire.ScheduleRunNowRequest) (wire.ScheduleRunNowResult, error) {
	sc, err := s.Store.Get(req.Project, req.Slug)
	switch {
	case errors.Is(err, schedrec.ErrNotFound):
		return wire.ScheduleRunNowResult{Refused: true, Reason: wire.ScheduleReasonUnknown, Message: "no schedule " + req.Project + ":" + req.Slug}, nil
	case err != nil:
		return wire.ScheduleRunNowResult{}, err
	}
	res, err := s.RunNow(sc)
	if err == nil && !res.Refused {
		s.publish(sc.Project, sc.Slug)
	}
	return res, err
}

func (s *Schedules) publish(project, slug string) {
	if s.Events != nil {
		s.Events.Publish(wire.EventSchedulesChanged, project+":"+slug)
	}
}

// written publishes schedules-changed and returns the Schedule's show payload.
func (s *Schedules) written(sc schedrec.Schedule) (wire.ScheduleShowResult, error) {
	s.publish(sc.Project, sc.Slug)
	registered, err := s.registered(sc)
	if err != nil {
		return wire.ScheduleShowResult{}, err
	}
	return wire.ScheduleShowResult{Schedule: s.info(sc, registered, s.now()), Prompt: sc.Prompt, LastRuns: []wire.ScheduleRun{}}, nil
}

// refusalOf maps a schedrec refusal to a result.
func refusalOf(err error) (wire.ScheduleShowResult, bool) {
	if reason := schedrec.ReasonOf(err); reason != "" {
		return refusedShow(reason, err.Error()), true
	}
	return wire.ScheduleShowResult{}, false
}
