package apiclient

import (
	"context"

	"github.com/elentok/gx/server/wire"
)

// Triage is the sub-client for Triage calls. It shares Client's transport.
type Triage struct {
	t *transport
}

// Changed pings the server that project's Triage record was written by a
// direct write, so the web hears of it at once.
func (s *Triage) Changed(ctx context.Context, project string) error {
	var res wire.TriageChangedResult
	return s.t.post(ctx, "/v1/triage/changed", wire.TriageChangedRequest{Project: project}, &res)
}
