package apiclient

import (
	"context"
	"net/url"

	"github.com/elentok/gx/server/wire"
)

// Triage is the sub-client for Triage calls. It shares Client's transport.
type Triage struct {
	t *transport
}

// Show reads project's Triage view.
func (s *Triage) Show(ctx context.Context, project string) (wire.TriageView, error) {
	var v wire.TriageView
	err := s.t.get(ctx, "/v1/triage?"+url.Values{"project": {project}}.Encode(), &v)
	return v, err
}

// Open starts project's Triage or returns the live session. A refusal is a
// result (Refused set), not an error.
func (s *Triage) Open(ctx context.Context, req wire.TriageOpenRequest) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/triage/open", req, &res)
	return res, err
}

// SaveRow stores a row's person fields. A refusal is a result, not an error.
func (s *Triage) SaveRow(ctx context.Context, req wire.TriageRowRequest) (wire.TriageEditResult, error) {
	var res wire.TriageEditResult
	err := s.t.post(ctx, "/v1/triage/row", req, &res)
	return res, err
}

// Abandon ends project's live Triage. A refusal is a result, not an error.
func (s *Triage) Abandon(ctx context.Context, req wire.TriageAbandonRequest) (wire.TriageEditResult, error) {
	var res wire.TriageEditResult
	err := s.t.post(ctx, "/v1/triage/abandon", req, &res)
	return res, err
}

// Changed pings the server that project's Triage record was written by a
// direct write, so the web hears of it at once.
func (s *Triage) Changed(ctx context.Context, project string) error {
	var res wire.TriageChangedResult
	return s.t.post(ctx, "/v1/triage/changed", wire.TriageChangedRequest{Project: project}, &res)
}
