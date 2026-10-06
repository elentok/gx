// Package apiclient is the CLI's (and test harness's) only way to call the
// orchestrator server.
package apiclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/elentok/gx/server"
)

type Client struct {
	http     *http.Client
	readOnly bool
}

// ErrReadOnly is returned by CheckWrite after Negotiate found an API version
// mismatch: the wire contract can't be trusted for writes.
var ErrReadOnly = errors.New("server API version differs from this client; run `gx server restart`")

// Negotiation is the outcome of comparing client and server versions.
type Negotiation struct {
	server.Handshake
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
	case h.APIVersion != server.APIVersion:
		n.ReadOnly = true
		n.Hint = fmt.Sprintf("read-only: server API v%d, client API v%d; run `gx server restart`", h.APIVersion, server.APIVersion)
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
func (c *Client) Handshake(ctx context.Context) (server.Handshake, error) {
	var h server.Handshake
	err := c.get(ctx, "/v1/handshake", &h)
	return h, err
}

// Snapshot returns every indexed ticket plus the server-wide sequence number.
func (c *Client) Snapshot(ctx context.Context) (server.Snapshot, error) {
	var s server.Snapshot
	err := c.get(ctx, "/v1/snapshot", &s)
	return s, err
}

// Events subscribes to the event stream from seq since (use Snapshot's Seq).
// The channel closes when the stream ends: the server dropped this client for
// lagging, stopped, or ctx was cancelled. Re-snapshot and resubscribe then.
func (c *Client) Events(ctx context.Context, since uint64) (<-chan server.Event, error) {
	path := fmt.Sprintf("/v1/events?since=%d", since)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://gx"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("server returned %s for %s", resp.Status, path)
	}
	out := make(chan server.Event)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			data, ok := strings.CutPrefix(sc.Text(), "data: ")
			if !ok {
				continue
			}
			var ev server.Event
			if json.Unmarshal([]byte(data), &ev) != nil {
				return
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
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
		return fmt.Errorf("server returned %s for %s", resp.Status, path)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
