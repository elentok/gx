package apiclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
)

// Snapshot returns every indexed ticket plus the server-wide sequence number.
func (c *Client) Snapshot(ctx context.Context) (wire.Snapshot, error) {
	var s wire.Snapshot
	err := c.get(ctx, "/v1/snapshot", &s)
	return s, err
}

// Events subscribes to the event stream from seq since (use Snapshot's Seq).
// The channel closes when the stream ends: the server dropped this client for
// lagging, stopped, or ctx was cancelled. Re-snapshot and resubscribe then.
func (c *Client) Events(ctx context.Context, since uint64) (<-chan server.Event, error) {
	return c.EventsAs(ctx, since, "")
}

// EventsAs is Events for a client that reports focus: its reports count
// while this stream is open.
func (c *Client) EventsAs(ctx context.Context, since uint64, client string) (<-chan server.Event, error) {
	path := fmt.Sprintf("/v1/events?since=%d", since)
	if client != "" {
		path += "&client=" + url.QueryEscape(client)
	}
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

// Follow keeps a consumer in sync: it takes a snapshot, then streams events
// from that snapshot's seq. Any stream end or seq gap (and any error while
// reconnecting) triggers a fresh snapshot, so the consumer never has to
// reason about missed events. Exactly one of the callback's arguments is
// non-nil. It returns only when ctx is done.
func (c *Client) Follow(ctx context.Context, fn func(*wire.Snapshot, *server.Event)) error {
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
func (c *Client) followOnce(ctx context.Context, fn func(*wire.Snapshot, *server.Event)) error {
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
