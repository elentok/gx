package servertest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

// SchedEpic is the epic every SchedFixture builds.
const SchedEpic = "epic"

// SchedFixture is one project with one epic, a server over it, and an agent
// whose turns are recorded in order. turn runs after the agent's commit.
type SchedFixture struct {
	Store, Repo string
	H           *Harness
	Issues      string
	mu          sync.Mutex
	order       []string
}

func NewSchedFixture(t *testing.T, tickets map[string]TicketOpts, agentTurn func(f *SchedFixture, p *Turn, id string), cfgOpts ...func(*server.Config)) *SchedFixture {
	t.Helper()
	f := &SchedFixture{Store: t.TempDir(), Repo: testutil.TempRepo(t)}
	for id, opts := range tickets {
		WriteTicketWith(t, f.Store, "proj", SchedEpic, id, "t", opts)
	}
	SetProjectRepo(t, f.Store, "proj", f.Repo)
	f.Issues = filepath.Join(f.Store, "proj", SchedEpic, "issues")
	f.H = StartWithStore(t, f.Store, func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
		c.MaxAgentsPerRoot = 1 // these scenarios pin the sequential order
		for _, o := range cfgOpts {
			o(c)
		}
	})
	if agentTurn == nil { // a runner of its own: no agent ever takes a turn
		return f
	}
	f.H.OnTurn(func(p *Turn) {
		_, id, _ := strings.Cut(p.Address, "/")
		f.mu.Lock()
		f.order = append(f.order, id)
		f.mu.Unlock()
		agentTurn(f, p, id)
	})
	return f
}

// CommitWork is the agent's turn: one commit named after the ticket, in its worktree.
func CommitWork(t *testing.T, p *Turn, name string) {
	t.Helper()
	testutil.WriteFile(t, p.Cwd, name+".txt", name)
	testutil.CommitAll(t, p.Cwd, "work "+name)
}

func (f *SchedFixture) Turns() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.order)
}

// Subscribe returns the event stream from now on.
func (f *SchedFixture) Subscribe(ctx context.Context, t *testing.T) <-chan wire.Event {
	t.Helper()
	snap, err := f.H.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := f.H.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

// QueueEpic queues the epic and returns the event stream from before the add.
func (f *SchedFixture) QueueEpic(ctx context.Context, t *testing.T, firstID string) <-chan wire.Event {
	t.Helper()
	evs := f.Subscribe(ctx, t)
	if res, err := f.H.Client.QueueAdd(ctx, "proj:"+SchedEpic+"/"+firstID, "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	return evs
}

func (f *SchedFixture) TicketFile(t *testing.T, id string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(f.Issues, id+"-*.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("ticket %s files = %v, %v", id, matches, err)
	}
	return matches[0]
}

func (f *SchedFixture) Git(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", f.Repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// GitOut runs git in dir and returns its trimmed output.
func GitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// Reload reloads the server's config and returns the result.
func Reload(t *testing.T, h *Harness) wire.ReloadResult {
	t.Helper()
	res, err := h.Client.ReloadConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// AssertMerged checks each ticket is done, its commit is on the feature branch
// and no iteration branch is left behind.
func (f *SchedFixture) AssertMerged(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if body := ReadFile(t, f.TicketFile(t, id)); !strings.Contains(body, "status: done") {
			t.Errorf("ticket %s not done:\n%s", id, body)
		}
		if got := f.Git(t, "show", SchedEpic+":"+id+".txt"); got != id {
			t.Errorf("feature branch %s:%s.txt = %q", SchedEpic, id, got)
		}
	}
	if left := strings.TrimSpace(f.Git(t, "branch", "--list", "ralph-loop/*")); left != "" {
		t.Errorf("iteration branches left behind:\n%s", left)
	}
}

// FeatureWorktree is the epic's feature worktree path.
func (f *SchedFixture) FeatureWorktree(t *testing.T) string {
	t.Helper()
	wtDir, err := ralphloop.DefaultDeps().WorktreeDir(f.Repo)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wtDir, SchedEpic)
}

// AssertFeatureKept checks the feature worktree and the epic branch survived.
func (f *SchedFixture) AssertFeatureKept(t *testing.T, when string) {
	t.Helper()
	if _, err := os.Stat(f.FeatureWorktree(t)); err != nil {
		t.Errorf("feature worktree gone after %s: %v", when, err)
	}
	if strings.TrimSpace(f.Git(t, "branch", "--list", SchedEpic)) == "" {
		t.Errorf("branch %s gone after %s", SchedEpic, when)
	}
}
