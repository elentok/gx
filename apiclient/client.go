// Package apiclient is the CLI's (and test harness's) only way to call the
// orchestrator server.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"

	"github.com/elentok/gx/server/wire"
)

type Client struct {
	http     *http.Client
	readOnly bool
	origin   string

	investigator string
}

// AsTUI marks the client's writes as coming from the TUI; an unmarked client
// writes as the CLI.
func (c *Client) AsTUI() *Client {
	c.origin = wire.OriginTUI
	return c
}

// AsInvestigator tags the client's writes as made by the investigator agent
// working on addr. A client without it reads the investigator from the process
// env, as the real CLI does inside an investigator session.
func (c *Client) AsInvestigator(addr string) *Client {
	c.investigator = addr
	return c
}

// setOrigin tags a write request with the client's origin.
func (c *Client) setOrigin(r *http.Request) {
	if c.origin != "" {
		r.Header.Set(wire.OriginHeader, c.origin)
	}
	addr := c.investigator
	if addr == "" {
		addr = os.Getenv(wire.InvestigateEnv)
	}
	if addr != "" {
		r.Header.Set(wire.InvestigateHeader, addr)
	}
}

// ErrReadOnly is returned by CheckWrite after Negotiate found an API version
// mismatch: the wire contract can't be trusted for writes.
var ErrReadOnly = errors.New("server API version differs from this client; run `gx server restart`")

// Negotiation is the outcome of comparing client and server versions.
type Negotiation struct {
	wire.Handshake
	ReadOnly bool
	// Hint is a user-facing remedy; empty when versions agree.
	Hint string
}

// Negotiate handshakes and compares versions. An API mismatch makes the client
// read-only; a build-only mismatch just yields a hint.
func (c *Client) Negotiate(ctx context.Context, build string) (Negotiation, error) {
	h, err := c.Handshake(ctx)
	if err != nil {
		return Negotiation{}, err
	}
	n := Negotiation{Handshake: h}
	switch {
	case h.APIVersion != wire.APIVersion:
		n.ReadOnly = true
		n.Hint = fmt.Sprintf("read-only: server API v%d, client API v%d; run `gx server restart`", h.APIVersion, wire.APIVersion)
	case h.Build != build:
		n.Hint = fmt.Sprintf("server is an older build (%s, client %s); run `gx server restart` to upgrade", h.Build, build)
	}
	c.readOnly = n.ReadOnly
	return n, nil
}

// CheckWrite must be called before any request that mutates server state.
func (c *Client) CheckWrite() error {
	if c.readOnly {
		return ErrReadOnly
	}
	return nil
}

// IsNotRunning reports whether err means nothing listens on the socket.
func IsNotRunning(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENOENT)
}

// New returns a client for the server listening on the unix socket.
func New(socketPath string) *Client {
	return &Client{http: &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}}}
}

// Handshake returns the server's API version and build id.
func (c *Client) Handshake(ctx context.Context) (wire.Handshake, error) {
	var h wire.Handshake
	err := c.get(ctx, "/v1/handshake", &h)
	return h, err
}

func (c *Client) post(ctx context.Context, path string, req, res any) error {
	if err := c.CheckWrite(); err != nil {
		return err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://gx"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	hreq.Header.Set("Content-Type", "application/json")
	c.setOrigin(hreq)
	resp, err := c.http.Do(hreq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned %s for %s: %s", resp.Status, path, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(res)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	// The host is ignored by the unix dialer; it only has to parse.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://gx"+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned %s for %s: %s", resp.Status, path, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
