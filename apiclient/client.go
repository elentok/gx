// Package apiclient is the CLI's (and test harness's) only way to call the
// orchestrator server.
package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/elentok/gx/server"
)

type Client struct {
	http *http.Client
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
