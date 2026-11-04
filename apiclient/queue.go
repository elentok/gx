package apiclient

import (
	"context"

	"github.com/elentok/gx/server"
)

// QueueAdd appends a ticket to the queue; agent "" takes the default.
func (c *Client) QueueAdd(ctx context.Context, address, agent string) (server.QueueResult, error) {
	return c.queueWrite(ctx, "add", server.QueueRequest{Address: address, Agent: agent})
}

// QueueRemove drops a ticket from the queue.
func (c *Client) QueueRemove(ctx context.Context, address string) (server.QueueResult, error) {
	return c.queueWrite(ctx, "remove", server.QueueRequest{Address: address})
}

// QueueReplace swaps one project's pending entries for items.
func (c *Client) QueueReplace(ctx context.Context, project string, items []server.QueueItem) (server.QueueResult, error) {
	return c.queueWrite(ctx, "replace", server.QueueRequest{Project: project, Items: items})
}

// QueueMove puts a queued ticket at the 1-based position.
func (c *Client) QueueMove(ctx context.Context, address string, position int) (server.QueueResult, error) {
	return c.queueWrite(ctx, "move", server.QueueRequest{Address: address, Position: position})
}

// QueuePause is Finish and pause: new roots stop starting and live runs finish.
// The pause is persisted across restarts.
func (c *Client) QueuePause(ctx context.Context) (server.QueueResult, error) {
	return c.queueWrite(ctx, "pause", server.QueueRequest{})
}

// QueueResume lifts a pause.
func (c *Client) QueueResume(ctx context.Context) (server.QueueResult, error) {
	return c.queueWrite(ctx, "resume", server.QueueRequest{})
}

// QueueDrain is an alias of QueuePause, kept for one release.
func (c *Client) QueueDrain(ctx context.Context) (server.QueueResult, error) {
	return c.queueWrite(ctx, "drain", server.QueueRequest{})
}

// queueWrite posts one queue write. A refusal is a result (Refused set), not an error.
func (c *Client) queueWrite(ctx context.Context, verb string, req server.QueueRequest) (server.QueueResult, error) {
	return c.queueWriteTo(ctx, "/v1/queue/"+verb, req)
}

func (c *Client) queueWriteTo(ctx context.Context, path string, req server.QueueRequest) (server.QueueResult, error) {
	var res server.QueueResult
	err := c.post(ctx, path, req, &res)
	return res, err
}
