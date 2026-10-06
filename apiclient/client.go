// Package apiclient is the CLI's (and test harness's) only way to call the
// orchestrator server.
package apiclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

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

// Projects returns every project in the server's ticket store with its state.
func (c *Client) Projects(ctx context.Context) ([]server.ProjectInfo, error) {
	var p []server.ProjectInfo
	err := c.get(ctx, "/v1/projects", &p)
	return p, err
}

// Locks returns every held lock with its owner.
func (c *Client) Locks(ctx context.Context) ([]server.LockInfo, error) {
	var l []server.LockInfo
	err := c.get(ctx, "/v1/locks", &l)
	return l, err
}

// History returns the events logged for one ticket, by its full address.
func (c *Client) History(ctx context.Context, address string) (server.History, error) {
	var h server.History
	err := c.get(ctx, "/v1/tickets/history?address="+url.QueryEscape(address), &h)
	return h, err
}

// Explain returns the scheduler's verdict on one ticket, by its full address.
func (c *Client) Explain(ctx context.Context, address string) (server.Explanation, error) {
	var e server.Explanation
	err := c.get(ctx, "/v1/tickets/explain?address="+url.QueryEscape(address), &e)
	return e, err
}

// Iterations returns the live iterations.
func (c *Client) Iterations(ctx context.Context) ([]server.IterationInfo, error) {
	var l []server.IterationInfo
	err := c.get(ctx, "/v1/iterations", &l)
	return l, err
}

// Queue returns every ticket with the scheduler's verdict on it.
func (c *Client) Queue(ctx context.Context) ([]server.QueueEntry, error) {
	var l []server.QueueEntry
	err := c.get(ctx, "/v1/queue", &l)
	return l, err
}

// QueueItems returns the server-wide queue in order.
func (c *Client) QueueItems(ctx context.Context) ([]server.QueueItem, error) {
	var l []server.QueueItem
	err := c.get(ctx, "/v1/queue/items", &l)
	return l, err
}

// QueueAdd appends a ticket to the queue; agent "" takes the default.
func (c *Client) QueueAdd(ctx context.Context, address, agent string) (server.QueueResult, error) {
	return c.queueWrite(ctx, "add", server.QueueRequest{Address: address, Agent: agent})
}

// QueueRemove drops a ticket from the queue.
func (c *Client) QueueRemove(ctx context.Context, address string) (server.QueueResult, error) {
	return c.queueWrite(ctx, "remove", server.QueueRequest{Address: address})
}

// QueueMove puts a queued ticket at the 1-based position.
func (c *Client) QueueMove(ctx context.Context, address string, position int) (server.QueueResult, error) {
	return c.queueWrite(ctx, "move", server.QueueRequest{Address: address, Position: position})
}

// TicketChanged pings the server that a direct write changed address's file.
func (c *Client) TicketChanged(ctx context.Context, address string) error {
	if err := c.CheckWrite(); err != nil {
		return err
	}
	body, err := json.Marshal(server.ChangedRequest{Address: address})
	if err != nil {
		return err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://gx/v1/tickets/changed", bytes.NewReader(body))
	if err != nil {
		return err
	}
	hreq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(hreq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("server returned %s for /v1/tickets/changed", resp.Status)
	}
	return nil
}

// queueWrite posts one queue write. A refusal is a result (Refused set), not an error.
func (c *Client) queueWrite(ctx context.Context, verb string, req server.QueueRequest) (server.QueueResult, error) {
	var res server.QueueResult
	err := c.post(ctx, "/v1/queue/"+verb, req, &res)
	return res, err
}

// Repair runs a repair verb ("land", "reset", "unpark" or "verify"). A refusal
// is a result (Refused set), not an error.
func (c *Client) Repair(ctx context.Context, verb string, req server.RepairRequest) (server.RepairResult, error) {
	var res server.RepairResult
	err := c.post(ctx, "/v1/tickets/"+verb, req, &res)
	return res, err
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

// Follow keeps a consumer in sync: it takes a snapshot, then streams events
// from that snapshot's seq. Any stream end or seq gap (and any error while
// reconnecting) triggers a fresh snapshot, so the consumer never has to
// reason about missed events. Exactly one of the callback's arguments is
// non-nil. It returns only when ctx is done.
func (c *Client) Follow(ctx context.Context, fn func(*server.Snapshot, *server.Event)) error {
	for {
		if err := c.followOnce(ctx, fn); err != nil && ctx.Err() == nil {
			select {
			case <-time.After(followRetry):
			case <-ctx.Done():
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

const followRetry = time.Second

// followOnce runs one snapshot+stream round and returns when it must restart.
func (c *Client) followOnce(ctx context.Context, fn func(*server.Snapshot, *server.Event)) error {
	snap, err := c.Snapshot(ctx)
	if err != nil {
		return err
	}
	fn(&snap, nil)
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	evs, err := c.Events(sctx, snap.Seq)
	if err != nil {
		return err
	}
	next := snap.Seq + 1
	for ev := range evs {
		if ev.Seq != next {
			return nil
		}
		next++
		fn(nil, &ev)
	}
	return nil
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
