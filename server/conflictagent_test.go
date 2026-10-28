package server_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

// turnRunner runs turn once per session, in its cwd, before the prompt
// settles: the agent's work.
type turnRunner struct {
	*runnerfake.Runner
	turn func(label, cwd string)

	mu     sync.Mutex
	cwds   map[string]string
	labels []string
	turned map[string]bool
}

func newTurnRunner(turn func(label, cwd string)) *turnRunner {
	r := &turnRunner{Runner: runnerfake.NewRunner(), turn: turn, cwds: map[string]string{}, turned: map[string]bool{}}
	r.PromptState = agentrunner.StateIdle
	return r
}

func (r *turnRunner) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	r.mu.Lock()
	r.cwds[opts.Label] = opts.Cwd
	r.labels = append(r.labels, opts.Label)
	r.mu.Unlock()
	return r.Runner.Start(opts)
}

func (r *turnRunner) Prompt(s agentrunner.Session, text string) error {
	r.mu.Lock()
	cwd, first := r.cwds[s.Label], !r.turned[s.Label]
	r.turned[s.Label] = true
	r.mu.Unlock()
	if first {
		r.turn(s.Label, cwd)
	}
	return r.Runner.Prompt(s, text)
}

func (r *turnRunner) started(prefix string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := slices.IndexFunc(r.labels, func(l string) bool { return strings.HasPrefix(l, prefix) })
	if i < 0 {
		return "", false
	}
	return r.labels[i], true
}

func writeProjectAgents(t *testing.T, store, repo, agents string) {
	t.Helper()
	body := `{"name":"proj","repo":"` + repo + `","auto-ff-merge":true` + agents + `}`
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

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
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.RunnerFor = func(_, _ string) agentrunner.Runner { return fake }
	})
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	ticketPath := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	deadline := time.Now().Add(20 * time.Second)
	var label string
	for ok := false; !ok; {
		if time.Now().After(deadline) {
			t.Fatal("no conflict agent started")
		}
		time.Sleep(20 * time.Millisecond)
		label, ok = fake.started("conflict-")
	}
	// Let the merge settle before teardown.
	for time.Now().Before(deadline) {
		if tk, err := schema.ParseTicket(ticketPath); err == nil && tk.Status != schema.StatusClaimed {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	iterLabel, _ := fake.started("epic-a-iter-")
	return fake.Args(iterLabel), fake.Args(label)
}

func hasFlag(args []string, flag, value string) bool {
	i := slices.Index(args, flag)
	return i >= 0 && i+1 < len(args) && args[i+1] == value
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

func TestConflictAgent_UsesTheProjectDefaultWhenTheParentRunHasNone(t *testing.T) {
	t.Parallel()
	iter, conflict := runToConflict(t, "", func(store, repo string) {
		writeProjectAgents(t, store, repo, `,"agents":{"claude":{"model":"opus","effort":"high"}}`)
	})
	if slices.Contains(iter, "--model") {
		t.Errorf("iteration args = %q, want no --model", iter)
	}
	if !hasFlag(conflict, "--model", "opus") || !hasFlag(conflict, "--effort", "high") {
		t.Errorf("conflict agent args = %q, want --model opus --effort high", conflict)
	}
}
