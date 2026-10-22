package ralphloop

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets/schema"
)

// gatedRunner blocks only the "wait for the agent to finish" Wait (the one
// whose states include done) until released, letting a test control exactly
// when each iteration completes and observe how many run concurrently in
// between. waitForFinish's confirmFinished re-waits the same session once
// more before treating it as genuinely finished, so a label's gate — once
// created — is reused (and passes straight through once released) rather
// than re-armed on every call: only the first call per label blocks and
// reports on started.
type gatedRunner struct {
	agentrunner.Runner
	// timeOut, when set, is asked first on every finish wait; true times that
	// wait out ungated, as an agent still at work.
	timeOut func(label string) bool
	// finishPollsOnly gates and times out only waitForFinish's own polls (the
	// ones that also stop on blocked), so smart-zone recovery's compact wait
	// passes straight through.
	finishPollsOnly bool
	// onInterrupt, when set, runs before every Interrupt.
	onInterrupt func(s agentrunner.Session)
	started     chan string

	mu    sync.Mutex
	gates map[string]chan struct{}
}

func (r *gatedRunner) Interrupt(s agentrunner.Session) error {
	if r.onInterrupt != nil {
		r.onInterrupt(s)
	}
	return r.Runner.Interrupt(s)
}

func newGatedRunner(next agentrunner.Runner) *gatedRunner {
	return &gatedRunner{Runner: next, started: make(chan string, 16), gates: map[string]chan struct{}{}}
}

func (r *gatedRunner) Wait(s agentrunner.Session, states []agentrunner.State, timeout time.Duration) (agentrunner.Status, error) {
	if !slices.Contains(states, agentrunner.StateDone) ||
		(r.finishPollsOnly && !slices.Contains(states, agentrunner.StateBlocked)) {
		return r.Runner.Wait(s, states, timeout)
	}
	if r.timeOut != nil && r.timeOut(s.Label) {
		return agentrunner.Status{State: agentrunner.StateWorking}, agentrunner.ErrTimeout
	}

	r.mu.Lock()
	gate, exists := r.gates[s.Label]
	if !exists {
		gate = make(chan struct{})
		r.gates[s.Label] = gate
	}
	r.mu.Unlock()

	if !exists {
		r.started <- s.Label
	}
	<-gate
	return r.Runner.Wait(s, states, timeout)
}

func (r *gatedRunner) release(label string) {
	r.mu.Lock()
	gate := r.gates[label]
	r.mu.Unlock()
	close(gate)
}

func TestRun_CherryPickConflict_ResolvesInFeatureWorktreeThenCompletes(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, prompts, removed := fakeDeps()

	var mu sync.Mutex
	var picks int
	var conflictPane, iterPane string
	var conflictPaneRemovedBefore bool
	inProgress := false

	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		mu.Lock()
		defer mu.Unlock()
		picks++
		if picks == 1 {
			inProgress = true
			return &fakeConflictErr{}
		}
		return nil
	}

	d.CherryPickInProgress = func(dir string) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		return inProgress, nil
	}

	onRunnerPrompt(d, func(s agentrunner.Session, text string) error {
		mu.Lock()
		defer mu.Unlock()
		if text == "/gx-resolving-merge-conflicts" {
			conflictPane = s.ID
			inProgress = false // resolution "commits", ending the cherry-pick sequence
		} else {
			iterPane = s.ID
		}
		return nil
	})

	origRemoveWorktree := d.RemoveWorktree
	d.RemoveWorktree = func(repoDir, path string, force bool) error {
		mu.Lock()
		if conflictPane == "" {
			conflictPaneRemovedBefore = true
		}
		mu.Unlock()
		return origRemoveWorktree(repoDir, path, force)
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if conflictPane == "" {
		t.Fatal("expected a conflict-resolution pane to be created")
	}
	if conflictPane == iterPane {
		t.Errorf("conflict-resolution pane %q must differ from the iteration pane %q (must run in the feature worktree)", conflictPane, iterPane)
	}
	if conflictPaneRemovedBefore {
		t.Error("iteration worktree was removed before the conflict-resolution pane was created")
	}

	if !slices.Contains(*prompts, "/gx-resolving-merge-conflicts") {
		t.Errorf("prompts = %v, want a /gx-resolving-merge-conflicts prompt", *prompts)
	}

	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the iteration worktree removed after resolution", *removed)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket not marked done after conflict resolution:\n%s", raw)
	}

	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}
	var gotTypes []string
	var conflictHit, conflictResolved *Event
	for i, ev := range events {
		gotTypes = append(gotTypes, ev.Type)
		switch ev.Type {
		case string(eventsc.ConflictHit):
			conflictHit = &events[i]
		case string(eventsc.ConflictResolved):
			conflictResolved = &events[i]
		}
	}
	if conflictHit == nil || conflictResolved == nil {
		t.Fatalf("event types = %v, want both %q and %q", gotTypes, string(eventsc.ConflictHit), string(eventsc.ConflictResolved))
	}
	if conflictHit.AgentSession == "" {
		t.Errorf("conflict-hit event = %+v, want a non-empty AgentSession (the iteration agent's own session)", conflictHit)
	}
	if conflictResolved.AgentSession == "" || conflictResolved.AgentSession == conflictHit.AgentSession {
		t.Errorf("conflict-resolved event = %+v, want a non-empty AgentSession distinct from conflict-hit's %q (the resolution agent's own session)", conflictResolved, conflictHit.AgentSession)
	}

	issuesDir := filepath.Join(scratchDir, "epic", "issues")
	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var childRaw string
	for _, e := range entries {
		if e.Name() == "01-a.md" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(issuesDir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if strings.Contains(string(raw), "type: conflict-resolution") {
			childRaw = string(raw)
			break
		}
	}
	if childRaw == "" {
		t.Fatalf("issues dir = %v, want a forked type: conflict-resolution child ticket for 01", entries)
	}
	if !strings.Contains(childRaw, "parent: \"01\"") && !strings.Contains(childRaw, "parent: 01") {
		t.Errorf("conflict-resolution child ticket missing parent: 01:\n%s", childRaw)
	}
	if !strings.Contains(childRaw, "status: done") {
		t.Errorf("conflict-resolution child ticket not marked done after successful resolution:\n%s", childRaw)
	}
}

func TestRun_AlreadyAppliedIteration_CompletesWithoutCherryPickOrResolver(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, prompts, _ := fakeDeps()
	d.PatchesApplied = func(dir, upstream, base, branch string) (bool, error) {
		return true, nil
	}
	var picks, trailerWrites int
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		picks++
		return nil
	}
	d.AppendTrailers = func(dir string, trailers ...git.Trailer) error {
		trailerWrites++
		return nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if picks != 0 {
		t.Errorf("CherryPickRange calls = %d, want 0 for already-applied patch", picks)
	}
	if trailerWrites != 0 {
		t.Errorf("AppendTrailers calls = %d, want 0 when no commit was created", trailerWrites)
	}
	if slices.Contains(*prompts, "/gx-resolving-merge-conflicts") {
		t.Errorf("prompts = %v, want no conflict resolver for already-applied patch", *prompts)
	}
	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket not marked done:\n%s", raw)
	}
}

func TestRun_StaleCherryPick_IsAbortedBeforeMergingCurrentTicket(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	stale := true
	var aborts, picks int
	d.CherryPickInProgress = func(dir string) (bool, error) { return stale, nil }
	d.AbortCherryPick = func(dir string) error {
		aborts++
		stale = false
		return nil
	}
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		if stale {
			t.Fatal("CherryPickRange called before stale state was aborted")
		}
		picks++
		return nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if aborts != 1 || picks != 1 {
		t.Errorf("aborts=%d picks=%d, want 1 and 1", aborts, picks)
	}
}

func TestRun_UnfinishedConflict_IsAbortedBeforeNextTicketMerges(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n# B\n",
	})
	d, _, _ := fakeDeps()
	active := false
	var aborts int
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		if strings.HasSuffix(toInclusive, "epic-item-01") {
			active = true
			return &fakeConflictErr{}
		}
		if active {
			t.Fatal("ticket 02 inherited ticket 01's cherry-pick")
		}
		return nil
	}
	d.CherryPickInProgress = func(dir string) (bool, error) { return active, nil }
	d.AbortCherryPick = func(dir string) error {
		aborts++
		active = false
		return nil
	}
	// Ticket 01's conflict leaves it paused (repair), so this run ends paused
	// on it rather than exiting; ticket 02 still has to merge first.
	runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo", MaxParallel: 1}, d, noopEventSink{})

	if aborts != 1 {
		t.Errorf("AbortCherryPick calls = %d, want 1", aborts)
	}
	raw02, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "02-b.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw02), "status: done") {
		t.Errorf("ticket 02 not marked done after ticket 01 cleanup:\n%s", raw02)
	}
}

func TestRun_CherryPickConflict_ResolutionNeverFinishes_MarksRepairWithoutAbortingRun(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, _ := fakeDeps()

	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		return &fakeConflictErr{}
	}
	d.CherryPickInProgress = func(dir string) (bool, error) {
		return true, nil // conflict never resolves
	}
	d.Runner = &blipRunner{Runner: fakeRunner(d), timeoutLabel: func(label string) bool {
		return strings.HasPrefix(label, "conflict-")
	}}
	// A stuck conflict resolution marks the ticket paused (repair) rather than
	// aborting the run, which leaves the epic paused on it.
	runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	assertRawPaused(t, string(raw), schema.PauseClassRepair, "")
}

// findConflictResolutionChild scans scratchDir/epicName/issues for the one
// forked "type: conflict-resolution" ticket, returning its raw file content.
func findConflictResolutionChild(t *testing.T, scratchDir, epicName string) string {
	t.Helper()
	issuesDir := filepath.Join(scratchDir, epicName, "issues")
	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(issuesDir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if strings.Contains(string(raw), "type: conflict-resolution") {
			return string(raw)
		}
	}
	t.Fatalf("issues dir = %v, want a forked type: conflict-resolution child ticket", entries)
	return ""
}

// TestRun_ConflictResolution_PrematureTabClose_SequencerStillConflicted_PausesOnChildNotParent
// reproduces the 2.1.228 incident's first sub-case: herdr reports the
// conflict-resolution agent's pane idle (a premature "done" signal — the
// agent hadn't actually finished), but the git sequencer is still genuinely
// conflicted. resolveCherryPickConflict must catch this via its own
// CherryPickInProgress corroboration rather than trusting herdr, and must
// pause paused (repair) on the conflict-resolution child ticket it forked —
// never on the parent iteration ticket, so a person sees exactly which
// conflict resolution needs attention instead of a generic iteration fault.
func TestRun_ConflictResolution_PrematureTabClose_SequencerStillConflicted_PausesOnChildNotParent(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, _ := fakeDeps()

	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		return &fakeConflictErr{}
	}
	// herdr's own AgentWait fake (from fakeDeps) reports the conflict-
	// resolution pane idle immediately — the premature "done" signal — while
	// this always reports the sequencer still conflicted, standing in for
	// the git ground truth herdr's signal disagreed with.
	d.CherryPickInProgress = func(dir string) (bool, error) {
		return true, nil
	}

	sink := &recordingSink{}
	runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)

	childRaw := findConflictResolutionChild(t, scratchDir, "epic")
	// Premature done signal with the sequencer still conflicted.
	assertRawPaused(t, childRaw, schema.PauseClassRepair, "")

	parentRaw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if rawIsPaused(t, string(parentRaw), schema.PauseClassRepair) {
		t.Errorf("parent iteration ticket = %q, want it left alone (not needs-repair) — the failure belongs to the conflict-resolution child", parentRaw)
	}

	// The parent is paused on its conflict-resolution child, not merged: its
	// on-disk status is still claimed (checked above via the absence of
	// paused (repair)), so Run must not report it as a completed iteration —
	// see mergeOne's pausedOnChild outcome and the results loop's handling of
	// it in loop.go.
	got := sink.snapshot()
	if slices.Contains(got, "IterationFinished") {
		t.Errorf("events = %v, want no IterationFinished for a ticket paused on its conflict-resolution child, not merged", got)
	}
}

// stopHookRunner calls onStop before every Stop.
type stopHookRunner struct {
	agentrunner.Runner
	onStop func(s agentrunner.Session)
}

func (r stopHookRunner) Stop(s agentrunner.Session) error {
	r.onStop(s)
	return r.Runner.Stop(s)
}

// TestRun_ConflictResolution_CorroboratesSequencerBeforeStoppingSession covers
// the ordering half of the 2.1.228 fix directly: resolveCherryPickConflict
// must check the git sequencer before trusting the agent's finished signal,
// not after. It fails the test if the conflict session is stopped before
// CherryPickInProgress has been consulted — the reverse of the original bug,
// where the tab closed (and the child ticket was marked done) before any
// sequencer check ran at all.
func TestRun_ConflictResolution_CorroboratesSequencerBeforeStoppingSession(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, _ := fakeDeps()

	inProgress := false
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		inProgress = true
		return &fakeConflictErr{}
	}

	var mu sync.Mutex
	onRunnerPrompt(d, func(_ agentrunner.Session, text string) error {
		if text == "/gx-resolving-merge-conflicts" {
			mu.Lock()
			inProgress = false
			mu.Unlock()
		}
		return nil
	})

	var sequencerCheckedBeforeStop bool
	d.CherryPickInProgress = func(dir string) (bool, error) {
		mu.Lock()
		sequencerCheckedBeforeStop = true
		cur := inProgress
		mu.Unlock()
		return cur, nil
	}
	d.Runner = stopHookRunner{Runner: d.Runner, onStop: func(s agentrunner.Session) {
		mu.Lock()
		checked := sequencerCheckedBeforeStop
		mu.Unlock()
		if strings.HasPrefix(s.Label, "conflict-") && !checked {
			t.Errorf("conflict-resolution session %s stopped before the sequencer was corroborated", s.Label)
		}
	}}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	childRaw := findConflictResolutionChild(t, scratchDir, "epic")
	if !strings.Contains(childRaw, "status: done") {
		t.Errorf("conflict-resolution child ticket = %q, want status: done once the sequencer corroborated the resolution", childRaw)
	}
}

// TestRun_RestartMidResolution_ReattachesLiveResolverWithoutReforking covers
// the run-level counterpart of conflict-lifecycle/02a's guard: a restart
// finds the cherry-pick sequencer already mid-conflict (from before the
// crash) with the conflict-resolution agent that was forked for it still
// live in its own tab. The fresh iteration launched for this ticket must
// reattach to that live resolver instead of aborting the stale sequencer
// state and forking a second one under the same conflict-labeled tab.
func TestRun_RestartMidResolution_ReattachesLiveResolverWithoutReforking(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md":                    "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
		"01a-conflict-resolution.md": "---\nid: \"01a\"\nstatus: claimed\ntype: conflict-resolution\nparent: \"01\"\n---\n# Conflict resolution for 01\n",
	})
	d, _, _ := fakeDeps()

	hostLiveAgent(t, d, "conflict-01")

	// The sequencer already owns a conflict from before the crash — no
	// CherryPickRange call for this ticket ever produces it. The first check
	// (inside cherryPickWithConflictResolution, before the live resolver is
	// found) sees it in progress; the second (inside reattachLiveConflictResolver,
	// after the reattached resolver finishes) sees it resolved.
	var cpChecks int32
	d.CherryPickInProgress = func(dir string) (bool, error) {
		return atomic.AddInt32(&cpChecks, 1) == 1, nil
	}

	var picks, aborts int32
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		atomic.AddInt32(&picks, 1)
		return nil
	}
	d.AbortCherryPick = func(dir string) error {
		atomic.AddInt32(&aborts, 1)
		return nil
	}

	var conflictStarts atomic.Int32
	onRunnerStart(d, func(opts agentrunner.StartOptions) error {
		if strings.HasPrefix(opts.Label, "conflict-") {
			conflictStarts.Add(1)
		}
		return nil
	})

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if atomic.LoadInt32(&aborts) != 0 {
		t.Errorf("AbortCherryPick calls = %d, want 0 (must reattach the live resolver, not abort its sequencer state)", aborts)
	}
	if n := conflictStarts.Load(); n != 0 {
		t.Errorf("conflict-labeled Runner starts = %d, want 0 (must reuse the live resolver, not fork a second one)", n)
	}
	if atomic.LoadInt32(&picks) != 0 {
		t.Errorf("CherryPickRange calls = %d, want 0 for this ticket (the sequencer was already mid-conflict)", picks)
	}

	issuesDir := filepath.Join(scratchDir, "epic", "issues")
	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var conflictChildren int
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(issuesDir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if strings.Contains(string(raw), "type: conflict-resolution") {
			conflictChildren++
		}
	}
	if conflictChildren != 1 {
		t.Errorf("conflict-resolution child ticket files = %d, want exactly 1 (no second fork)", conflictChildren)
	}

	childRaw := findConflictResolutionChild(t, scratchDir, "epic")
	if !strings.Contains(childRaw, "status: done") {
		t.Errorf("conflict-resolution child ticket = %q, want status: done once the reattached resolver finished", childRaw)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("parent ticket = %q, want status: done", raw)
	}
}

// fakeConflictErr stands in for the *git.RunError CherryPickRange returns on
// a real conflict; only its presence (not its type) matters to the loop,
// which distinguishes conflicts from other errors via CherryPickInProgress.
type fakeConflictErr struct{}

func (e *fakeConflictErr) Error() string { return "cherry-pick conflict" }

func TestRun_ZeroCommitIteration_MarksAskingAndLeavesWorktree(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, removed := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}

	sink := &recordingSink{}
	// The zero-commit iteration leaves its ticket paused (answer), so the epic's
	// only ticket is one a human must clear: the run pauses on it.
	runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)

	if len(*removed) != 0 {
		t.Errorf("removed worktree branches = %v, want the zero-commit iteration's worktree left in place", *removed)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	assertRawPaused(t, string(raw), schema.PauseClassAnswer, schema.PauseZeroCommit)
	if strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket must not be marked done after a zero-commit iteration:\n%s", raw)
	}

	// A paused ticket already gets its own notification via
	// TicketNeedsHuman; it must not also fire IterationFinished (the
	// spurious "done" notification) or count toward completed — see
	// loop.go's paused early-continue in the results-handling loop.
	got := sink.snapshot()
	if !slices.Contains(got, "TicketNeedsHuman") {
		t.Errorf("events = %v, want TicketNeedsHuman for the paused ticket", got)
	}
	if slices.Contains(got, "IterationFinished") {
		t.Errorf("events = %v, want no IterationFinished for a paused (zero-commit) ticket", got)
	}
}

func TestRun_ZeroCommitIteration_ReasonNamesADirtyWorktree(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		dirty []git.Change
		want  string
	}{
		{"clean", nil, "no commits merged"},
		{"dirty", []git.Change{{Path: "a"}, {Path: "b"}, {Path: "c"}}, "no commits merged; worktree has uncommitted changes (3 files)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scratchDir := writeEpic(t, "epic", map[string]string{
				"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
			})
			d, _, _ := fakeDeps()
			d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) { return 0, nil }
			d.UncommittedChanges = func(string) ([]git.Change, error) { return tc.dirty, nil }
			sink := &recordingSink{}
			runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)
			if len(sink.ticketNeedsHumanCalls) != 1 || sink.ticketNeedsHumanCalls[0][3] != tc.want {
				t.Errorf("TicketNeedsHuman calls = %v, want one with reason %q", sink.ticketNeedsHumanCalls, tc.want)
			}
		})
	}
}

func TestRun_ZeroCommitIteration_OtherTicketsStillMerge(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n# B\n",
	})
	d, _, removed := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		// The zero-commit pause's clearable check asks about the iteration
		// branch from the feature worktree.
		if strings.Contains(dir, "epic-item-01") || strings.HasSuffix(toRef, "epic-item-01") {
			return 0, nil
		}
		return 1, nil
	}

	// Ticket 01 ends paused (answer), so the run pauses on it once 02 has merged.
	runUntilPaused(t, RunOptions{
		EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
		MaxParallel: 1,
	}, d, noopEventSink{})

	if len(*removed) != 1 || (*removed)[0] != "ralph-loop/epic-item-02" {
		t.Errorf("removed worktree branches = %v, want only iter-02 removed", *removed)
	}

	raw01, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	assertRawPaused(t, string(raw01), schema.PauseClassAnswer, "")

	raw02, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "02-b.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw02), "status: done") {
		t.Errorf("ticket 02 not marked done:\n%s", raw02)
	}
}

// TestRun_TransientIdleBlip_DoesNotOrphanCommit reproduces the ticket-05
// incident: herdr reported the agent idle for one poll, then it went back to
// work and committed shortly after. waitForFinish's confirmFinished recheck
// (loop.go) should catch that the first idle signal didn't hold and keep
// waiting instead of the loop marking the ticket paused (answer) and abandoning a
// worktree that was about to merge a commit.
func TestRun_TransientIdleBlip_DoesNotOrphanCommit(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, removed := fakeDeps()

	// The second wait is the debounce recheck: the agent went back to work in
	// the meantime, so this "confirm" wait should see it still busy.
	r := &blipRunner{Runner: fakeRunner(d), timeoutOn: 2}
	d.Runner = r

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if r.waits < 3 {
		t.Fatalf("Runner.Wait calls = %d, want at least 3 (initial idle, failed confirm, real finish)", r.waits)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket not marked done despite the agent finishing after the transient blip:\n%s", raw)
	}
	if rawIsPaused(t, string(raw), schema.PauseClassAnswer) {
		t.Errorf("ticket wrongly marked needs-answer from a transient idle blip:\n%s", raw)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the iteration's worktree removed", *removed)
	}
}

// TestRun_CommitMergesDuringAskingRecheck_MarksDoneNotAsking covers
// finishIteration's own recheck (loop.go): even after waitForFinish's
// confirmFinished settles on "finished", a commit can still merge in the
// window before CommitsAhead is checked (e.g. a reattached iteration, which
// skips waitForFinish's debounce). The recheck should catch it instead of
// orphaning the ticket as paused (answer).
func TestRun_CommitMergesDuringAskingRecheck_MarksDoneNotAsking(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, removed := fakeDeps()

	var calls int32
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return 0, nil
		}
		return 1, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket not marked done after commit merged on recheck:\n%s", raw)
	}
	if rawIsPaused(t, string(raw), schema.PauseClassAnswer) {
		t.Errorf("ticket wrongly marked needs-answer despite commit merging on recheck:\n%s", raw)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the iteration's worktree removed", *removed)
	}
}
