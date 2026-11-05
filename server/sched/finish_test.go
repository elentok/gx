package sched_test

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

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/runnerbuild"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

func TestRunner_ClaimsWriteTheFileBeforeTheEventAndLaunchWithTheChosenAgent(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	claimedOnDisk := false
	for ev := range evs {
		if ev.Type == wire.EventTicketClaimed {
			data, err := os.ReadFile(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
			if err != nil {
				t.Fatal(err)
			}
			claimedOnDisk = strings.Contains(string(data), "status: claimed")
			if ev.Address != "proj:epic-a/01" {
				t.Errorf("claim event address = %q", ev.Address)
			}
		}
		if ev.Type == wire.EventIterationStarted {
			break
		}
	}
	if !claimedOnDisk {
		t.Error("ticket file did not say claimed when the claim event streamed")
	}
	const label = "epic-a-iter-01"
	if got := fake.Kind(label); got != "codex" {
		t.Errorf("agent kind = %q, want codex", got)
	}
	if got := strings.Join(fake.Prompts(label), " "); !strings.Contains(got, "$gx-implement proj:epic-a/01") {
		t.Errorf("agent prompts = %v, want the codex skill prompt", got)
	}
	if runs := h.Server.Runs(); len(runs) != 1 || runs[0].Address != "proj:epic-a/01" || runs[0].Agent != "codex" {
		t.Errorf("runs = %+v", runs)
	}
}

// gx-implement asks for commits, which a code-review ticket never makes.
func TestRunner_CodeReviewTicketLaunchesUnderTheCodeReviewSkill(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "review", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-review.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "type: implement", "type: code-review", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store)
	_, prompt, _ := servertest.RegisterTurn(h)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for ev := range evs {
		if ev.Type == wire.EventIterationStarted {
			break
		}
	}
	if got := strings.Join(prompt(), " "); !strings.Contains(got, "/gx-code-review proj:epic-a/01") {
		t.Errorf("agent prompt = %v, want the gx-code-review skill", got)
	}
}

func TestRunner_LaunchSkillFollowsTicketType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ typ, want string }{
		{"prompt", "/gx-one-off proj:epic-a/01"},
		{"implement", "/gx-implement proj:epic-a/01"},
		{"evolve", "/gx-evolve proj:epic-a/01"},
		{"diagnose", "/gx-investigate proj:epic-a/01"},
		{"investigate", "/gx-investigate proj:epic-a/01"},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{Type: tc.typ})
			servertest.SetProjectRepo(t, store, "proj", repo)
			h := servertest.StartWithStore(t, store)
			_, prompt, _ := servertest.RegisterTurn(h)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			snap, err := h.Client.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			evs, err := h.Client.Events(ctx, snap.Seq)
			if err != nil {
				t.Fatal(err)
			}
			if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
				t.Fatalf("add: %+v, %v", res, err)
			}
			for ev := range evs {
				if ev.Type == wire.EventIterationStarted {
					break
				}
			}
			if got := strings.Join(prompt(), " "); !strings.Contains(got, tc.want) {
				t.Errorf("agent prompt = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

func TestRunner_MergesTheIterationAndClosesTheRoot(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	// The agent's turn: commit a file in its own worktree.
	h.OnTurn(func(turn *servertest.Turn) {
		testutil.WriteFile(t, turn.Cwd, "agent.txt", "work")
		testutil.CommitAll(t, turn.Cwd, "agent work")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	var seen []string
	for ev := range evs {
		switch ev.Type {
		case wire.EventTicketClaimed, wire.EventIterationStarted, wire.EventTicketDone, wire.EventRootCompleted,
			wire.EventIterationFailed, wire.EventIterationPaused:
			seen = append(seen, ev.Type)
		}
		if ev.Type == wire.EventRootCompleted || ev.Type == wire.EventIterationFailed {
			break
		}
	}
	want := []string{wire.EventTicketClaimed, wire.EventIterationStarted, wire.EventTicketDone, wire.EventRootCompleted}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", seen, want)
	}
	data, err := os.ReadFile(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "status: done") {
		t.Errorf("ticket file is not done:\n%s", data)
	}
	out, err := exec.Command("git", "-C", repo, "show", "epic-a:agent.txt").CombinedOutput()
	if err != nil || string(out) != "work" {
		t.Errorf("feature branch lacks the agent's commit: %q, %v", out, err)
	}
}

func TestRunner_AFinishThatErrorsPausesTheTicketStuckAndFreesTheRoot(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	// The agent's turn wrecks its own worktree, so the finish cannot read it.
	fake := servertest.NewTurnRunner(func(_, cwd string) {
		if err := os.RemoveAll(cwd); err != nil {
			t.Error(err)
		}
	})
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for ev := range evs {
		if ev.Type == wire.EventIterationFailed {
			break
		}
	}
	tk, err := schema.ParseTicket(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertPaused(t, tk, schema.PauseClassRepair, schema.PauseIterationError)
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Errorf("runs = %+v, want the root freed", runs)
	}
}

// A claim that pauses on an ambiguous base launches nothing, so claimNext must
// not count it against the concurrency limit. The frontier never offers such a
// ticket (it shares the blocking rule with base derivation), so the claim is
// driven directly.
func TestRunner_AmbiguousBasePausesWithoutLaunching(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "03", "third", servertest.TicketOpts{BlockedBy: []string{"01", "02"}})
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	servertest.RegisterTurn(h)

	launched, err := h.Server.Scheduler().ClaimTicket("proj:epic-a/03", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if launched {
		t.Error("claim reported launched, want paused with no slot used")
	}
	if runs := h.Server.Runs(); len(runs) != 0 {
		t.Errorf("runs = %+v, want none", runs)
	}
	body, err := os.ReadFile(filepath.Join(store, "proj", "epic-a", "issues", "03-third.md"))
	if err != nil || !strings.Contains(string(body), "status: paused") || !strings.Contains(string(body), "pause_reason: ambiguous-base") {
		t.Errorf("ticket 03 (%v):\n%s\nwant status: paused, pause_reason: ambiguous-base", err, body)
	}
}

func TestRunner_RestartReclaimsTheLiveIterationAndItFinishes(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, servertest.WithHerdr(servertest.HerdrIsTheSubject))
	registerLiveAgent(h)
	leaveIteration(t, h, store, repo, "herdr")

	h.Restart(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	for ev := range evs {
		switch ev.Type {
		case wire.EventReclaimed, wire.EventIterationStarted, wire.EventTicketDone, wire.EventRootCompleted,
			wire.EventIterationFailed, wire.EventIterationPaused:
			seen = append(seen, ev.Type)
		}
		if ev.Type == wire.EventRootCompleted || ev.Type == wire.EventIterationFailed {
			break
		}
	}
	want := []string{wire.EventReclaimed, wire.EventTicketDone, wire.EventRootCompleted}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		log, _ := os.ReadFile(server.LogPath(h.StateDir))
		t.Fatalf("events = %v, want %v\nserver log:\n%s", seen, want, log)
	}
	// The merge writes under the project, where the manual merge and the
	// tickets live, never under a phantom <store>/<epic>.
	logged, _, err := runlog.ReadEvents(filepath.Join(store, "proj"), "epic-a")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(logged, func(ev runlog.Event) bool { return ev.Type == string(events.CherryPicked) }) {
		t.Errorf("no cherry-picked event in the project's run log: %v", logged)
	}
	if _, err := os.Stat(filepath.Join(store, "epic-a")); !os.IsNotExist(err) {
		t.Errorf("merge wrote under the store root: stat %s: %v", filepath.Join(store, "epic-a"), err)
	}
}

func TestRunner_LaunchesAndRecordsThroughTheConfiguredRunner(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := runnerfake.NewRunner()
	var asked []string
	var mu sync.Mutex
	h := servertest.StartWithStore(t, store, servertest.WithRunnerFor(fake, func(_, project string) agentrunner.Runner {
		mu.Lock()
		defer mu.Unlock()
		asked = append(asked, project)
		return fake
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for ev := range evs {
		if ev.Type == wire.EventIterationStarted {
			break
		}
	}
	runs := h.Server.Runs()
	if len(runs) != 1 || runs[0].Runner != "fake" {
		t.Fatalf("runs = %+v, want one recorded as fake", runs)
	}
	// End the agent's turn so its run settles before the harness tears down.
	t.Cleanup(func() { fake.SetState(runs[0].Session.Label, agentrunner.StateDone, "") })
	if _, ok, _ := fake.Find(runs[0].Session.Label); !ok {
		t.Error("the agent was not started on the configured runner")
	}
	mu.Lock()
	defer mu.Unlock()
	for _, p := range asked {
		// scratch is asked by the log prune, which visits every project.
		// The herdr health probe is asked for the fallback project.
		if p != "proj" && p != projects.ScratchProject && p != runnerbuild.FallbackProject {
			t.Errorf("RunnerFor asked for project %q", p)
		}
	}
}

func TestRunner_CodexEvolveLaunchGetsTheFollowUpsAndGxDirs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, typ, agent string
		wantExtra        bool
	}{
		{"codex evolve", "evolve", "codex", true},
		{"codex implement", "implement", "codex", false},
		{"claude evolve", "evolve", "claude", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, repo := t.TempDir(), testutil.TempRepo(t)
			servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{Type: tc.typ})
			servertest.WriteTicket(t, store, "gx", "other", "01", "unrelated", "")
			servertest.SetProjectRepo(t, store, "proj", repo)
			fake := runnerfake.NewRunner()
			h := servertest.StartWithStore(t, store, servertest.WithRunner(fake))
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			snap, err := h.Client.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			evs, err := h.Client.Events(ctx, snap.Seq)
			if err != nil {
				t.Fatal(err)
			}
			if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", tc.agent); err != nil || res.Refused {
				t.Fatalf("add: %+v, %v", res, err)
			}
			for ev := range evs {
				if ev.Type == wire.EventIterationStarted {
					break
				}
			}
			got := strings.Join(fake.Args("epic-a-iter-01"), " ")
			if strings.Contains(got, "--add-dir") {
				t.Errorf("--add-dir in %q", got)
			}
			// Codex roots are quoted entries of one writable_roots array.
			epicDir := `"` + filepath.Join(store, "proj", "epic-a") + `"`
			if has := strings.Contains(got, epicDir); has != (tc.agent == "codex") {
				t.Errorf("%s present = %v in %q", epicDir, has, got)
			}
			for _, dir := range []string{filepath.Join(store, "gx", "follow-ups"), filepath.Join(store, "gx")} {
				if has := strings.Contains(got, `"`+dir+`"`); has != tc.wantExtra {
					t.Errorf("root %s present = %v, want %v in %q", dir, has, tc.wantExtra, got)
				}
			}
		})
	}
}
