package schedules

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	schedrec "github.com/elentok/gx/schedules"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

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

func (s *Schedules) registeredProject(project string) (bool, error) {
	if project == projects.ScratchProject {
		return true, nil
	}
	names, err := s.Projects()
	return slices.Contains(names, project), err
}

// lateLimit applies a requested Late limit; a refusal result is returned when
// it is not above 0 (Validate would silently default a 0).
func lateLimit(sc *schedrec.Schedule, hours *float64) (ops.ScheduleShowResult, bool) {
	if hours == nil {
		return ops.ScheduleShowResult{}, false
	}
	if !(*hours > 0) {
		return refusedShow(schedrec.ReasonInvalidLateLimit, "late limit must be above 0"), true
	}
	sc.LateLimitHours = *hours
	return ops.ScheduleShowResult{}, false
}

func (s *Schedules) create(req ops.ScheduleCreateRequest) (ops.ScheduleShowResult, error) {
	project := req.Project
	if project == "" {
		project = projects.ScratchProject
	}
	ok, err := s.registeredProject(project)
	if err != nil {
		return ops.ScheduleShowResult{}, err
	}
	if !ok {
		return refusedShow(schedrec.ReasonInvalidProject, "no project "+project), nil
	}
	days, err := schedrec.ParseDays(req.Days)
	if err != nil {
		return refusedShow(schedrec.ReasonNoDays, err.Error()), nil
	}
	sc := schedrec.Schedule{Name: req.Name, Prompt: req.Prompt, Days: days, Times: req.Times, Harness: req.Harness, Tier: req.Tier}
	if res, refused := lateLimit(&sc, req.LateLimitHours); refused {
		return res, nil
	}
	created, err := s.Store.Create(project, sc)
	if res, handled := refusalOf(err); handled {
		return res, nil
	} else if err != nil {
		return ops.ScheduleShowResult{}, err
	}
	// A new Schedule is on here only; other machines show it "Off here".
	if err := s.State.Enable(Key{Project: created.Project, Slug: created.Slug}, created.CreatedAt); err != nil {
		return ops.ScheduleShowResult{}, err
	}
	it, err := s.Store.Get(project, created.Slug)
	if err != nil {
		return ops.ScheduleShowResult{}, err
	}
	return s.written(it)
}

func (s *Schedules) edit(req ops.ScheduleEditRequest) (ops.ScheduleShowResult, error) {
	if req.NewProject != nil && *req.NewProject != "" && *req.NewProject != req.Project {
		return refusedShow(ops.ScheduleReasonProjectReadOnly, "the project of a Schedule cannot change"), nil
	}
	it, reason, msg, err := s.getOrRefuse(req.Project, req.Slug)
	switch {
	case err != nil:
		return ops.ScheduleShowResult{}, err
	case reason != "":
		return refusedShow(reason, msg), nil
	}
	sc := it.Schedule
	if req.Name != nil {
		sc.Name = *req.Name
	}
	if req.Prompt != nil {
		sc.Prompt = *req.Prompt
	}
	if req.Days != nil {
		days, err := schedrec.ParseDays(req.Days)
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
	if req.Tier != nil {
		sc.Tier = *req.Tier
	}
	if res, refused := lateLimit(&sc, req.LateLimitHours); refused {
		return res, nil
	}
	updated, err := s.Store.Update(req.Project, req.Slug, sc)
	if res, handled := refusalOf(err); handled {
		return res, nil
	} else if err != nil {
		return ops.ScheduleShowResult{}, err
	}
	// The edit applies at Save: no earlier time runs on this machine.
	if err := s.State.Advance(Key{Project: updated.Project, Slug: updated.Slug}, updated.UpdatedAt); err != nil {
		return ops.ScheduleShowResult{}, err
	}
	return s.written(schedrec.Item{Schedule: updated, Registered: it.Registered})
}

// switchTo writes only this machine's state, never the Schedule file. Turning
// on sets the baseline to now, so time spent off never runs; turning off
// leaves a queued run alone.
func (s *Schedules) switchTo(req ops.ScheduleSwitchRequest) (ops.ScheduleShowResult, error) {
	it, reason, msg, err := s.getOrRefuse(req.Project, req.Slug)
	switch {
	case err != nil:
		return ops.ScheduleShowResult{}, err
	case reason != "":
		return refusedShow(reason, msg), nil
	}
	k := Key{Project: it.Project, Slug: it.Slug}
	if req.On {
		err = s.State.Enable(k, s.now())
	} else {
		err = s.State.Disable(k)
	}
	if err != nil {
		return ops.ScheduleShowResult{}, err
	}
	return s.written(it)
}

// remove deletes the Schedule file, then runs one check so this machine's own
// queued runs are cancelled and its state dropped without waiting for a tick.
func (s *Schedules) remove(req ops.ScheduleDeleteRequest) (ops.ScheduleDeleteResult, error) {
	err := s.Store.Delete(req.Project, req.Slug)
	switch {
	case errors.Is(err, schedrec.ErrNotFound):
		return ops.ScheduleDeleteResult{Refused: true, Reason: ops.ScheduleReasonUnknown, Message: "no schedule " + req.Project + ":" + req.Slug}, nil
	case err != nil:
		if reason := schedrec.ReasonOf(err); reason != "" {
			return ops.ScheduleDeleteResult{Refused: true, Reason: reason, Message: err.Error()}, nil
		}
		return ops.ScheduleDeleteResult{}, err
	}
	if s.Check != nil {
		s.Check()
	}
	s.publish(req.Project, req.Slug)
	return ops.ScheduleDeleteResult{}, nil
}

// runNow starts one run of the Schedule at once, whatever the switch says.
func (s *Schedules) runNow(req ops.ScheduleRunNowRequest) (ops.ScheduleRunNowResult, error) {
	it, reason, msg, err := s.getOrRefuse(req.Project, req.Slug)
	switch {
	case err != nil:
		return ops.ScheduleRunNowResult{}, err
	case reason != "":
		return ops.ScheduleRunNowResult{Refused: true, Reason: reason, Message: msg}, nil
	}
	res, err := s.RunNow(it.Schedule, it.Registered)
	if err == nil && !res.Refused {
		s.publish(it.Project, it.Slug)
	}
	return res, err
}

func (s *Schedules) publish(project, slug string) {
	if s.Events != nil {
		s.Events.Publish(ops.EventSchedulesChanged, project+":"+slug)
	}
}

// written publishes schedules-changed and returns the Schedule's show payload.
func (s *Schedules) written(it schedrec.Item) (ops.ScheduleShowResult, error) {
	s.publish(it.Project, it.Slug)
	return ops.ScheduleShowResult{Schedule: s.info(it.Schedule, it.Registered, s.now()), Prompt: it.Prompt, LastRuns: []ops.ScheduleRun{}}, nil
}

// refusalOf maps a schedrec refusal to a result.
func refusalOf(err error) (ops.ScheduleShowResult, bool) {
	if reason := schedrec.ReasonOf(err); reason != "" {
		return refusedShow(reason, err.Error()), true
	}
	return ops.ScheduleShowResult{}, false
}

// getOrRefuse loads a Schedule. A missing Schedule or a schedrec refusal
// comes back as a (reason, message) pair for the handler to map to its result
// type; any other failure is err.
func (s *Schedules) getOrRefuse(project, slug string) (it schedrec.Item, reason, message string, err error) {
	it, err = s.Store.Get(project, slug)
	switch {
	case errors.Is(err, schedrec.ErrNotFound):
		return it, ops.ScheduleReasonUnknown, "no schedule " + project + ":" + slug, nil
	case err != nil:
		if r := schedrec.ReasonOf(err); r != "" {
			return it, r, err.Error(), nil
		}
	}
	return it, "", "", err
}
