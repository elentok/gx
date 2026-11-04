package apiclient

import (
	"context"

	"github.com/elentok/gx/server"
)

// WebInfo returns the web listener's base URL, root fingerprint and the names
// its leaf leaves out; a refusal when the listener is off.
func (c *Client) WebInfo(ctx context.Context) (server.WebInfo, error) {
	var info server.WebInfo
	err := c.get(ctx, "/v1/web/info", &info)
	return info, err
}

// WebNewRoot makes a new web root and swaps it and its leaf live. A refusal is
// a result (Refused set), not an error.
func (c *Client) WebNewRoot(ctx context.Context) (server.WebInfo, error) {
	var info server.WebInfo
	err := c.post(ctx, "/v1/web/new-root", struct{}{}, &info)
	return info, err
}

// WebReset clears the web account and returns the new Setup code in the result.
// A refusal is a result (Refused set), not an error.
func (c *Client) WebReset(ctx context.Context) (server.WebInfo, error) {
	var info server.WebInfo
	err := c.post(ctx, "/v1/web/reset", struct{}{}, &info)
	return info, err
}

// WebLogins lists the web logins. A refusal is a result (Refused set), not an error.
func (c *Client) WebLogins(ctx context.Context) (server.WebLoginsResult, error) {
	var res server.WebLoginsResult
	err := c.get(ctx, "/v1/web/login-list", &res)
	return res, err
}

// WebRevokeLogin revokes one web login by id. A refusal is a result, not an error.
func (c *Client) WebRevokeLogin(ctx context.Context, id string) (server.WebLoginsResult, error) {
	var res server.WebLoginsResult
	err := c.post(ctx, "/v1/web/login-revoke", struct {
		ID string `json:"id"`
	}{id}, &res)
	return res, err
}

// WebSetPassword saves a new web password and signs every web login out. A
// refusal is a result, not an error.
func (c *Client) WebSetPassword(ctx context.Context, password string) (server.WebLoginsResult, error) {
	var res server.WebLoginsResult
	err := c.post(ctx, "/v1/web/set-password", struct {
		Password string `json:"password"`
	}{password}, &res)
	return res, err
}

// WebResetAuthenticator clears the authenticator and returns the new Setup code in the result.
// A refusal is a result (Refused set), not an error.
func (c *Client) WebResetAuthenticator(ctx context.Context) (server.WebInfo, error) {
	var info server.WebInfo
	err := c.post(ctx, "/v1/web/reset-authenticator", struct{}{}, &info)
	return info, err
}
