package servertest

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

// WaitFor polls cond every few milliseconds until it holds, failing the test
// with "timed out waiting for <what>" after timeout.
func WaitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	if !Poll(timeout, cond) {
		t.Fatalf("timed out waiting for %s", what)
	}
}

// Poll is WaitFor without the failure: it reports whether cond held in time.
func Poll(timeout time.Duration, cond func() bool) bool {
	for deadline := time.Now().Add(timeout); !cond(); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			return false
		}
	}
	return true
}

// WaitForEvent returns the first streamed event of typ for address (any
// address when empty), failing the test if the stream ends or ctx expires first.
func WaitForEvent(ctx context.Context, t *testing.T, evs <-chan server.Event, typ, address string) server.Event {
	t.Helper()
	for {
		select {
		case ev, open := <-evs:
			if !open {
				t.Fatalf("event stream ended before %s %s", typ, address)
			}
			if ev.Type == typ && (address == "" || ev.Address == address) {
				return ev
			}
		case <-ctx.Done():
			t.Fatalf("no %s event for %q: %v", typ, address, ctx.Err())
		}
	}
}
