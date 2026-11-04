package servertest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
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

// WaitMigrations fails the test unless h's start-up migrations end in time.
func WaitMigrations(t *testing.T, h *Harness) {
	t.Helper()
	select {
	case <-h.Server.MigrationsDone():
	case <-time.After(10 * time.Second):
		t.Fatal("migrations did not end")
	}
}

// HeldMigration returns a Config option whose migration step blocks until the
// returned release is called, and a channel closed once the step is running.
func HeldMigration(t *testing.T) (opt func(*server.Config), entered <-chan struct{}, release func()) {
	t.Helper()
	in := make(chan struct{})
	gate := make(chan struct{})
	var once, relOnce sync.Once
	reg := &migrations.Registry{}
	reg.Register(migrations.Step{Name: "hold", Run: func(migrations.Env) ([]tickets.MigrationChange, error) {
		once.Do(func() { close(in) })
		<-gate
		return nil, nil
	}})
	release = func() { relOnce.Do(func() { close(gate) }) }
	t.Cleanup(release)
	return func(c *server.Config) { c.Migrations = reg }, in, release
}

// WaitForEvent returns the first streamed event of typ for address (any
// address when empty), failing the test if the stream ends or ctx expires first.
func WaitForEvent(ctx context.Context, t *testing.T, evs <-chan wire.Event, typ, address string) wire.Event {
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
