package ticketops_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

var (
	writeProjectAgents = servertest.WriteProjectAgents
	hasFlag            = servertest.HasFlag
)

// runToConflict launches proj:epic-a/01 whose commit conflicts with the feature
// tip, calls midRun during the agent's turn, and returns the iteration's and the
// conflict agent's launch args.
func runToConflict(t *testing.T, agents string, midRun func(store, repo string)) (iter, conflict []string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	writeProjectAgents(t, store, repo, agents)
	fake := newTurnRunner(func(label, cwd string) {
		if strings.HasPrefix(label, "conflict-") {
			return
		}
		testutil.WriteFile(t, cwd, "agent.txt", "mine")
		testutil.CommitAll(t, cwd, "work")
		feature := filepath.Join(filepath.Dir(cwd), "epic-a")
		testutil.WriteFile(t, feature, "agent.txt", "theirs")
		testutil.CommitAll(t, feature, "conflicting")
		midRun(store, repo)
	})
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake))
	if res, err := h.Client.Queue.Add(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	ticketPath := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	var label string
	servertest.WaitFor(t, 20*time.Second, "a conflict agent to start", func() bool {
		var ok bool
		label, ok = fake.Started("conflict-")
		return ok
	})
	// Let the merge settle before teardown.
	servertest.Poll(20*time.Second, func() bool {
		tk, err := schema.ParseTicket(ticketPath)
		return err == nil && tk.Status != schema.StatusClaimed
	})
	iterLabel, _ := fake.Started("epic-a-iter-")
	return fake.Args(iterLabel), fake.Args(label)
}

func TestConflictAgent_InheritsTheParentRunsModelAndEffort(t *testing.T) {
	t.Parallel()
	iter, conflict := runToConflict(t, `,"agents":{"claude":{"model":"opus","effort":"high"}}`, func(store, repo string) {
		writeProjectAgents(t, store, repo, `,"agents":{"claude":{"model":"haiku","effort":"low"}}`)
	})
	for name, args := range map[string][]string{"iteration": iter, "conflict agent": conflict} {
		if !hasFlag(args, "--model", "opus") || !hasFlag(args, "--effort", "high") {
			t.Errorf("%s args = %q, want --model opus --effort high", name, args)
		}
	}
}
