package launchtest_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

// inflightRig is a server whose launches stop in the install step until
// release, so claimed tickets stay launching.
type inflightRig struct {
	h       *servertest.Harness
	ctx     context.Context
	store   string
	release chan struct{}
}

// ticketPath is where startInflight wrote ticket id of epic.
func (r *inflightRig) ticketPath(epic, id string) string {
	return filepath.Join(r.store, "proj", epic, "issues", id+"-t"+id+".md")
}

// startInflight writes epics (epic -> ticket ids) into one project with extra
// project.json members, and holds every launch's install until the rig ends.
// failFor names an epic whose install fails at once instead.
func startInflight(t *testing.T, epics map[string][]string, projectExtra, failFor string, cfg func(*server.Config)) *inflightRig {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	for epic, ids := range epics {
		for _, id := range ids {
			servertest.WriteTicket(t, store, "proj", epic, id, "t"+id, "")
		}
	}
	if err := servertest.WriteProjectRepo(store, "proj", repo, projectExtra); err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		cfg = func(*server.Config) {}
	}
	r := &inflightRig{store: store, release: make(chan struct{})}
	t.Cleanup(func() { close(r.release) })
	r.h = servertest.StartWithStore(t, store, cfg, func(c *server.Config) {
		c.InstallDeps = func(ctx context.Context, path, _ string) ([]string, error) {
			if failFor != "" && strings.Contains(path, failFor) {
				return nil, errors.New("boom")
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-r.release:
				return nil, nil
			}
		}
	})
	var cancel context.CancelFunc
	r.ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return r
}

func (r *inflightRig) queue(t *testing.T, addrs ...string) {
	t.Helper()
	for _, addr := range addrs {
		if res, err := r.h.Client.Queue.Add(r.ctx, addr, "claude"); err != nil || res.Refused {
			t.Fatalf("add %s: %+v, %v", addr, res, err)
		}
	}
}

// Story 14: running plus launching never passes the epic, project or global cap.
func TestClaimNext_EachCapBindsWhileLaunchesAreInFlight(t *testing.T) {
	t.Parallel()
	threeRoots := map[string][]string{"epic-a": {"01"}, "epic-b": {"01"}, "epic-c": {"01"}}
	cases := map[string]struct {
		epics  map[string][]string
		extra  string
		cfg    func(*server.Config)
		queued []string
	}{
		"epic cap":    {map[string][]string{"epic-a": {"01", "02", "03"}}, "", func(c *server.Config) { c.MaxAgents, c.MaxAgentsPerRoot = 5, 2 }, []string{"proj:epic-a/01"}},
		"project cap": {threeRoots, `,"max-agents":2`, func(c *server.Config) { c.MaxAgents = 5 }, []string{"proj:epic-a/01", "proj:epic-b/01", "proj:epic-c/01"}},
		"global cap":  {threeRoots, "", func(c *server.Config) { c.MaxAgents = 2 }, []string{"proj:epic-a/01", "proj:epic-b/01", "proj:epic-c/01"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := startInflight(t, tc.epics, tc.extra, "", tc.cfg)
			r.queue(t, tc.queued...)
			if !servertest.Poll(30*time.Second, func() bool { return r.h.Server.Registry().Count() == 2 }) {
				t.Fatalf("runs = %d, want the cap of 2 launching", r.h.Server.Registry().Count())
			}
			time.Sleep(500 * time.Millisecond) // a wrong third claim would land by now
			if n := r.h.Server.Registry().Count(); n != 2 {
				t.Fatalf("runs = %d after settling, want the cap of 2", n)
			}
		})
	}
}

// Story 9: the index reads the ticket claimed while its launch is still running.
func TestClaim_TheIndexReadsClaimedWhileTheLaunchIsHeld(t *testing.T) {
	t.Parallel()
	r := startInflight(t, map[string][]string{"epic-a": {"01"}}, "", "", nil)
	r.queue(t, "proj:epic-a/01")
	if !servertest.Poll(30*time.Second, func() bool {
		snap, err := r.h.Client.Snapshot(r.ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, ti := range snap.Tickets {
			if ti.Address == "proj:epic-a/01" && ti.Status == string(schema.StatusClaimed) {
				return true
			}
		}
		return false
	}) {
		t.Fatal("the index never read the ticket as claimed")
	}
	if n := r.h.Server.Registry().Count(); n != 1 {
		t.Errorf("registry = %d, want the one held launch", n)
	}
}

// Story 15: a failed launch pauses with its kind and frees its slot, and the
// next queued ticket launches into it.
func TestLaunchFailure_PausesFreesTheSlotAndTheNextTicketLaunches(t *testing.T) {
	t.Parallel()
	r := startInflight(t, map[string][]string{"epic-a": {"01"}, "epic-b": {"01"}}, "", "epic-a", func(c *server.Config) { c.MaxAgents = 1 })
	snap, err := r.h.Client.Snapshot(r.ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := r.h.Client.Events(r.ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	r.queue(t, "proj:epic-a/01", "proj:epic-b/01")
	servertest.WaitForEvent(r.ctx, t, evs, wire.EventIterationLaunchFailed, "proj:epic-a/01")
	servertest.WaitStatus(t, r.ticketPath("epic-a", "01"), schema.StatusPaused)
	if tk, err := schema.ParseTicket(r.ticketPath("epic-a", "01")); err != nil || string(tk.PauseReason) != string(events.IterationError) {
		t.Errorf("pause reason = %q (%v), want %q", tk.PauseReason, err, events.IterationError)
	}
	servertest.WaitStatus(t, r.ticketPath("epic-b", "01"), schema.StatusClaimed)
	if !servertest.Poll(30*time.Second, func() bool { return r.h.Server.Registry().Count() == 1 }) {
		t.Errorf("registry = %d, want only the second ticket's launch", r.h.Server.Registry().Count())
	}
}
