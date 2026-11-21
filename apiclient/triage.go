package apiclient

import (
	"context"
	"net/url"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
)

// Triage is the sub-client for Triage calls. It shares Client's transport.
type Triage struct {
	t *transport
}

// Show reads project's Triage view.
func (s *Triage) Show(ctx context.Context, project string) (ops.TriageView, error) {
	var v ops.TriageView
	err := s.t.get(ctx, "/v1/triage?"+url.Values{"project": {project}}.Encode(), &v)
	return v, err
}

// Open starts project's Triage or returns the live session. A refusal is a
// result (Refused set), not an error.
func (s *Triage) Open(ctx context.Context, req ops.TriageOpenRequest) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/triage/open", req, &res)
	return res, err
}

// Ungroup undoes the group a row is in. A refusal is a result, not an error.
func (s *Triage) Ungroup(ctx context.Context, req ops.TriageUngroupRequest) (ops.TriageEditResult, error) {
	var res ops.TriageEditResult
	err := s.t.post(ctx, "/v1/triage/ungroup", req, &res)
	return res, err
}

// Abandon ends project's live Triage. A refusal is a result, not an error.
func (s *Triage) Abandon(ctx context.Context, req ops.TriageAbandonRequest) (ops.TriageEditResult, error) {
	var res ops.TriageEditResult
	err := s.t.post(ctx, "/v1/triage/abandon", req, &res)
	return res, err
}

// Retry sets a failed row pending and starts a Write session unless one is live.
func (s *Triage) Retry(ctx context.Context, req ops.TriageRowActionRequest) (ops.TriageEditResult, error) {
	var res ops.TriageEditResult
	err := s.t.post(ctx, "/v1/triage/retry", req, &res)
	return res, err
}

// Keep marks a failed row kept for later.
func (s *Triage) Keep(ctx context.Context, req ops.TriageRowActionRequest) (ops.TriageEditResult, error) {
	var res ops.TriageEditResult
	err := s.t.post(ctx, "/v1/triage/keep", req, &res)
	return res, err
}

// Changed pings the server that project's Triage record was written by a
// direct write, so the web hears of it at once.
func (s *Triage) Changed(ctx context.Context, project string) error {
	var res ops.TriageChangedResult
	return s.t.post(ctx, "/v1/triage/changed", ops.TriageChangedRequest{Project: project}, &res)
}
