package tickets

import (
	"context"
	"slices"
	"strings"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets/schema"
)

// QueueAddFunc is the queue-add call of the server client.
type QueueAddFunc func(ctx context.Context, address, agent string) (server.QueueResult, error)

// QueueAddresses sends one queue-add per address, carrying on past refusals so
// one blocked ticket doesn't keep the rest out. It returns how many the server
// took and the refusals as "address: reason".
func QueueAddresses(ctx context.Context, add QueueAddFunc, addrs []string) (queued int, refusals []string) {
	for _, addr := range addrs {
		res, err := add(ctx, addr, "")
		switch {
		case err != nil:
			refusals = append(refusals, addr+": "+err.Error())
		case res.Refused:
			refusals = append(refusals, addr+": "+res.Message)
		default:
			queued++
		}
	}
	return queued, refusals
}

// QueueEpic queues, in address order, the open tickets of epic ("project:epic")
// found in snap, with QueueAddresses' counts.
func QueueEpic(ctx context.Context, snap server.Snapshot, add QueueAddFunc, epic string) (queued int, refusals []string) {
	prefix := epic + "/"
	var addrs []string
	for _, t := range snap.Tickets {
		if strings.HasPrefix(t.Address, prefix) && t.Status == string(schema.StatusOpen) {
			addrs = append(addrs, t.Address)
		}
	}
	slices.Sort(addrs)
	return QueueAddresses(ctx, add, addrs)
}
