package apiclient

import (
	"context"
	"net/url"

	"github.com/elentok/gx/server/wire"
)

// Schedules is the sub-client for stored Schedules. It shares Client's transport.
type Schedules struct {
	t *transport
}

// List returns every Schedule and the registered project names.
func (s *Schedules) List(ctx context.Context) (wire.SchedulesResult, error) {
	var res wire.SchedulesResult
	err := s.t.get(ctx, "/v1/schedules", &res)
	return res, err
}

// Create stores a new Schedule, on here only. A refusal is a result.
func (s *Schedules) Create(ctx context.Context, req wire.ScheduleCreateRequest) (wire.ScheduleShowResult, error) {
	var res wire.ScheduleShowResult
	err := s.t.post(ctx, "/v1/schedules/create", req, &res)
	return res, err
}

// Edit changes the fields given on an existing Schedule. A refusal is a result.
func (s *Schedules) Edit(ctx context.Context, req wire.ScheduleEditRequest) (wire.ScheduleShowResult, error) {
	var res wire.ScheduleShowResult
	err := s.t.post(ctx, "/v1/schedules/edit", req, &res)
	return res, err
}

// Switch turns a Schedule on or off on this machine. A refusal is a result.
func (s *Schedules) Switch(ctx context.Context, req wire.ScheduleSwitchRequest) (wire.ScheduleShowResult, error) {
	var res wire.ScheduleShowResult
	err := s.t.post(ctx, "/v1/schedules/switch", req, &res)
	return res, err
}

// Delete removes a Schedule. A refusal is a result.
func (s *Schedules) Delete(ctx context.Context, req wire.ScheduleDeleteRequest) (wire.ScheduleDeleteResult, error) {
	var res wire.ScheduleDeleteResult
	err := s.t.post(ctx, "/v1/schedules/delete", req, &res)
	return res, err
}

// RunNow starts one run of a Schedule now. A refusal is a result.
func (s *Schedules) RunNow(ctx context.Context, req wire.ScheduleRunNowRequest) (wire.ScheduleRunNowResult, error) {
	var res wire.ScheduleRunNowResult
	err := s.t.post(ctx, "/v1/schedules/run-now", req, &res)
	return res, err
}

// Show returns one Schedule with its prompt and Last runs. An unknown one is a
// refusal result, not an error.
func (s *Schedules) Show(ctx context.Context, project, slug string) (wire.ScheduleShowResult, error) {
	var res wire.ScheduleShowResult
	err := s.t.get(ctx, "/v1/schedules/show?"+url.Values{"project": {project}, "slug": {slug}}.Encode(), &res)
	return res, err
}
