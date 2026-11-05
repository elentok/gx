package apiclient

import (
	"context"

	"github.com/elentok/gx/server/wire"
)

// Web is the sub-client for the web account and listener. It shares Client's
// transport.
type Web struct {
	t *transport
}

// Info returns the web listener's base URL, root fingerprint and the names
// its leaf leaves out; a refusal when the listener is off.
func (w *Web) Info(ctx context.Context) (wire.WebInfo, error) {
	var info wire.WebInfo
	err := w.t.get(ctx, "/v1/web/info", &info)
	return info, err
}

// NewRoot makes a new web root and swaps it and its leaf live. A refusal is a
// result (Refused set), not an error.
func (w *Web) NewRoot(ctx context.Context) (wire.WebInfo, error) {
	var info wire.WebInfo
	err := w.t.post(ctx, "/v1/web/new-root", struct{}{}, &info)
	return info, err
}

// Reset clears the web account and returns the new Setup code in the result.
// A refusal is a result (Refused set), not an error.
func (w *Web) Reset(ctx context.Context) (wire.WebInfo, error) {
	var info wire.WebInfo
	err := w.t.post(ctx, "/v1/web/reset", struct{}{}, &info)
	return info, err
}

// Logins lists the web logins. A refusal is a result (Refused set), not an error.
func (w *Web) Logins(ctx context.Context) (wire.WebLoginsResult, error) {
	var res wire.WebLoginsResult
	err := w.t.get(ctx, "/v1/web/login-list", &res)
	return res, err
}

// RevokeLogin revokes one web login by id. A refusal is a result, not an error.
func (w *Web) RevokeLogin(ctx context.Context, id string) (wire.WebLoginsResult, error) {
	var res wire.WebLoginsResult
	err := w.t.post(ctx, "/v1/web/login-revoke", struct {
		ID string `json:"id"`
	}{id}, &res)
	return res, err
}

// SetPassword saves a new web password and signs every web login out. A
// refusal is a result, not an error.
func (w *Web) SetPassword(ctx context.Context, password string) (wire.WebLoginsResult, error) {
	var res wire.WebLoginsResult
	err := w.t.post(ctx, "/v1/web/set-password", struct {
		Password string `json:"password"`
	}{password}, &res)
	return res, err
}

// ResetAuthenticator clears the authenticator and returns the new Setup code in the result.
// A refusal is a result (Refused set), not an error.
func (w *Web) ResetAuthenticator(ctx context.Context) (wire.WebInfo, error) {
	var info wire.WebInfo
	err := w.t.post(ctx, "/v1/web/reset-authenticator", struct{}{}, &info)
	return info, err
}
