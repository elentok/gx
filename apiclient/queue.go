package apiclient

import (
	"context"

	"github.com/elentok/gx/server/wire"
)

// Queue is the sub-client for the queue and one-off tickets. It shares
// Client's transport.
type Queue struct {
	t *transport
}

// Add appends a ticket to the queue; agent "" takes the default.
func (q *Queue) Add(ctx context.Context, address, agent string) (wire.QueueResult, error) {
	return q.write(ctx, "add", wire.QueueRequest{Address: address, Agent: agent})
}

// List returns every ticket with the scheduler's verdict on it.
func (q *Queue) List(ctx context.Context) ([]wire.QueueEntry, error) {
	var l []wire.QueueEntry
	err := q.t.get(ctx, "/v1/queue", &l)
	return l, err
}

// Items returns the server-wide queue in order.
func (q *Queue) Items(ctx context.Context) ([]wire.QueueItem, error) {
	var l []wire.QueueItem
	err := q.t.get(ctx, "/v1/queue/items", &l)
	return l, err
}

// OneOff creates a top-level ticket from a prompt and queues it. A refusal is a
// result (Refused set), not an error.
func (q *Queue) OneOff(ctx context.Context, req wire.OneOffRequest) (wire.OneOffResult, error) {
	var res wire.OneOffResult
	err := q.t.post(ctx, "/v1/oneoff", req, &res)
	return res, err
}

// Remove drops a ticket from the queue.
func (q *Queue) Remove(ctx context.Context, address string) (wire.QueueResult, error) {
	return q.write(ctx, "remove", wire.QueueRequest{Address: address})
}

// Replace swaps one project's pending entries for items.
func (q *Queue) Replace(ctx context.Context, project string, items []wire.QueueItem) (wire.QueueResult, error) {
	return q.write(ctx, "replace", wire.QueueRequest{Project: project, Items: items})
}

// Move puts a queued ticket at the 1-based position.
func (q *Queue) Move(ctx context.Context, address string, position int) (wire.QueueResult, error) {
	return q.write(ctx, "move", wire.QueueRequest{Address: address, Position: position})
}

// Pause is Finish and pause: new roots stop starting and live runs finish.
// The pause is persisted across restarts.
func (q *Queue) Pause(ctx context.Context) (wire.QueueResult, error) {
	return q.write(ctx, "pause", wire.QueueRequest{})
}

// Resume lifts a pause.
func (q *Queue) Resume(ctx context.Context) (wire.QueueResult, error) {
	return q.write(ctx, "resume", wire.QueueRequest{})
}

// Drain is an alias of Pause, kept for one release.
func (q *Queue) Drain(ctx context.Context) (wire.QueueResult, error) {
	return q.write(ctx, "drain", wire.QueueRequest{})
}

// write posts one queue write. A refusal is a result (Refused set), not an error.
func (q *Queue) write(ctx context.Context, verb string, req wire.QueueRequest) (wire.QueueResult, error) {
	return q.t.queueWriteTo(ctx, "/v1/queue/"+verb, req)
}

func (t *transport) queueWriteTo(ctx context.Context, path string, req wire.QueueRequest) (wire.QueueResult, error) {
	var res wire.QueueResult
	err := t.post(ctx, path, req, &res)
	return res, err
}
