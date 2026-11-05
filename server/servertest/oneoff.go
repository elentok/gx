package servertest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// StartOneOffHarness starts a paused server with one project (proj, repo
// returned) holding the epic-a root ticket.
func StartOneOffHarness(t *testing.T, opts ...func(*server.Config)) (*Harness, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	SetProjectRepo(t, store, "proj", repo)
	h := StartWithStore(t, store, append(opts, func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
	})...)
	if _, err := h.Client.Queue.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	return h, repo
}

// StartOneOffsHarness starts a server over store whose one-offs epic may run
// two agents at once while a root runs one.
func StartOneOffsHarness(t *testing.T, store string, opts ...func(*server.Config)) *Harness {
	t.Helper()
	SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	return StartWithStore(t, store, append(opts, func(c *server.Config) {
		c.MaxAgentsPerRoot = 1
		c.MaxOneOffAgents = 2
		c.MaxAgents = 10
		c.PollInterval = 50 * time.Millisecond
	})...)
}

// WaitForAgentKinds returns the kind of each of the one-off agents (labelled
// one-offs-iter-<id>) once all of them have started.
func WaitForAgentKinds(t *testing.T, fake *runnerfake.Runner, ids ...string) map[string]agentrunner.Kind {
	t.Helper()
	var kinds map[string]agentrunner.Kind
	WaitFor(t, 10*time.Second, fmt.Sprintf("%d agent starts", len(ids)), func() bool {
		kinds = map[string]agentrunner.Kind{}
		for _, id := range ids {
			label := "one-offs-iter-" + id
			if k := fake.Kind(label); k != "" {
				kinds[label] = k
			}
		}
		return len(kinds) == len(ids)
	})
	return kinds
}
