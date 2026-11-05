package apiclient

import (
	"context"

	"github.com/elentok/gx/server/wire"
)

// QueueAdd appends a ticket to the queue; agent "" takes the default.
func (c *Client) QueueAdd(ctx context.Context, address, agent string) (wire.QueueResult, error) {
	return c.queueWrite(ctx, "add", wire.QueueRequest{Address: address, Agent: agent})
}

// QueueRemove drops a ticket from the queue.
func (c *Client) QueueRemove(ctx context.Context, address string) (wire.QueueResult, error) {
	return c.queueWrite(ctx, "remove", wire.QueueRequest{Address: address})
}

// QueueReplace swaps one project's pending entries for items.
func (c *Client) QueueReplace(ctx context.Context, project string, items []wire.QueueItem) (wire.QueueResult, error) {
	return c.queueWrite(ctx, "replace", wire.QueueRequest{Project: project, Items: items})
}

// QueueMove puts a queued ticket at the 1-based position.
func (c *Client) QueueMove(ctx context.Context, address string, position int) (wire.QueueResult, error) {
	return c.queueWrite(ctx, "move", wire.QueueRequest{Address: address, Position: position})
}

// QueuePause is Finish and pause: new roots stop starting and live runs finish.
// The pause is persisted across restarts.
func (c *Client) QueuePause(ctx context.Context) (wire.QueueResult, error) {
	return c.queueWrite(ctx, "pause", wire.QueueRequest{})
}

// QueueResume lifts a pause.
func (c *Client) QueueResume(ctx context.Context) (wire.QueueResult, error) {
	return c.queueWrite(ctx, "resume", wire.QueueRequest{})
}

// QueueDrain is an alias of QueuePause, kept for one release.
func (c *Client) QueueDrain(ctx context.Context) (wire.QueueResult, error) {
	return c.queueWrite(ctx, "drain", wire.QueueRequest{})
}

// queueWrite posts one queue write. A refusal is a result (Refused set), not an error.
func (c *Client) queueWrite(ctx context.Context, verb string, req wire.QueueRequest) (wire.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/queue/"+verb, req)
}

func (t *transport) queueWriteTo(ctx context.Context, path string, req wire.QueueRequest) (wire.QueueResult, error) {
	var res wire.QueueResult
	err := t.post(ctx, path, req, &res)
	return res, err
}
