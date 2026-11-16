package apiclient

import (
	"context"

	"github.com/elentok/gx/server/wire"
)

// Rest is the sub-client for the Rest engine. It shares Client's transport.
type Rest struct{ t *transport }

// Status reads the Rest status.
func (r *Rest) Status(ctx context.Context) (wire.RestStatus, error) {
	var res wire.RestStatus
	err := r.t.get(ctx, "/v1/rest", &res)
	return res, err
}

// Active reports one active minute. A refusal is a result, not an error.
func (r *Rest) Active(ctx context.Context) (wire.RestResult, error) {
	var res wire.RestResult
	err := r.t.post(ctx, "/v1/rest/active", struct{}{}, &res)
	return res, err
}

// Unlock tries to end the running Rest period with the typed sentence. A wrong
// sentence is a refusal result, not an error.
func (r *Rest) Unlock(ctx context.Context, sentence string) (wire.RestResult, error) {
	var res wire.RestResult
	err := r.t.post(ctx, "/v1/rest/unlock", wire.RestUnlockRequest{Sentence: sentence}, &res)
	return res, err
}
