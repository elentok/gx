package ralphloop

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
)

// gatedAgentWait wraps a fakeDeps' AgentWait so that only the "wait for the
// agent to finish" call (the one whose Until includes "done") blocks until
// released, letting a test control exactly when each iteration completes and
// observe how many run concurrently in between. waitForFinish's
// confirmFinished re-polls the same pane once more (with the same Until list)
// before treating it as genuinely finished, so a pane's gate — once created —
// is reused (and returns immediately once released) rather than re-armed on
// every call: only the first call per pane blocks and reports on started.
func gatedAgentWait(next func(herdr.AgentWaitOptions) (herdr.Agent, error)) (
	wait func(herdr.AgentWaitOptions) (herdr.Agent, error),
	started <-chan string,
	release func(pane string),
) {
	var mu sync.Mutex
	gates := map[string]chan struct{}{}
	startedCh := make(chan string, 16)

	wait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		isFinish := false
		for _, u := range opts.Until {
			if u == "done" {
				isFinish = true
			}
		}
		if !isFinish {
			return next(opts)
		}

		mu.Lock()
		gate, exists := gates[opts.Target]
		if !exists {
			gate = make(chan struct{})
			gates[opts.Target] = gate
		}
		mu.Unlock()

		if !exists {
			startedCh <- opts.Target
		}
		<-gate
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}

	release = func(pane string) {
		mu.Lock()
		gate := gates[pane]
		mu.Unlock()
		close(gate)
	}

	return wait, startedCh, release
}

func TestRun_CherryPickConflict_ResolvesInFeatureWorktreeThenCompletes(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, prompts, removed := fakeDeps()
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

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

	origTabCreate := d.TabCreate
	d.TabCreate = func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error) {
		mu.Lock()
		conflictPane = "pane-" + opts.Label
		mu.Unlock()
		return origTabCreate(opts)
	}

	origAgentPrompt := d.AgentPrompt
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if opts.Text == "/gx-resolving-merge-conflicts" {
			mu.Lock()
			inProgress = false // resolution "commits", ending the cherry-pick sequence
			mu.Unlock()
		} else {
			mu.Lock()
			iterPane = opts.Target
			mu.Unlock()
		}
		return origAgentPrompt(opts)
	}

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

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	var gotTypes []string
	var conflictHit, conflictResolved *Event
	for i, ev := range events {
		gotTypes = append(gotTypes, ev.Type)
		switch ev.Type {
		case eventConflictHit:
			conflictHit = &events[i]
		case eventConflictResolved:
			conflictResolved = &events[i]
		}
	}
	if conflictHit == nil || conflictResolved == nil {
		t.Fatalf("event types = %v, want both %q and %q", gotTypes, eventConflictHit, eventConflictResolved)
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
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
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

func TestRun_StaleCherryPick_IsAbortedBeforeLandingCurrentTicket(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
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

func TestRun_UnfinishedConflict_IsAbortedBeforeNextTicketLands(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
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
	// Ticket 01's conflict leaves it needs-repair, so this run ends parked
	// on it rather than exiting; ticket 02 still has to land first.
	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo", MaxParallel: 1}, d, noopEventSink{})

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

func TestRun_CherryPickConflict_ResolutionNeverFinishes_MarksNeedsRepairWithoutAbortingRun(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()

	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		return &fakeConflictErr{}
	}
	d.CherryPickInProgress = func(dir string) (bool, error) {
		return true, nil // conflict never resolves
	}
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		isFinish := slices.Contains(opts.Until, "done")
		isConflictPane := strings.HasPrefix(opts.Target, "pane-conflict-")
		if isFinish && isConflictPane {
			return herdr.Agent{}, errors.New("timeout waiting for agent")
		}
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}
	// A stuck conflict resolution marks the ticket needs-repair rather than
	// aborting the run, which leaves the epic parked on it.
	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-repair") {
		t.Errorf("ticket file = %q, want Status: needs-repair after the stuck conflict resolution", raw)
	}
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

// TestRun_ConflictResolution_PrematureTabClose_SequencerStillConflicted_ParksOnChildNotParent
// reproduces the 2.1.228 incident's first sub-case: herdr reports the
// conflict-resolution agent's pane idle (a premature "done" signal — the
// agent hadn't actually finished), but the git sequencer is still genuinely
// conflicted. resolveCherryPickConflict must catch this via its own
// CherryPickInProgress corroboration rather than trusting herdr, and must
// park needs-repair on the conflict-resolution child ticket it forked —
// never on the parent iteration ticket, so a person sees exactly which
// conflict resolution needs attention instead of a generic iteration fault.
func TestRun_ConflictResolution_PrematureTabClose_SequencerStillConflicted_ParksOnChildNotParent(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
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

	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})

	childRaw := findConflictResolutionChild(t, scratchDir, "epic")
	if !strings.Contains(childRaw, "status: needs-repair") {
		t.Errorf("conflict-resolution child ticket = %q, want status: needs-repair after a premature done signal with the sequencer still conflicted", childRaw)
	}

	parentRaw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(parentRaw), "status: needs-repair") {
		t.Errorf("parent iteration ticket = %q, want it left alone (not needs-repair) — the failure belongs to the conflict-resolution child", parentRaw)
	}
}

// TestRun_ConflictResolution_CorroboratesSequencerBeforeClosingTab covers the
// ordering half of the 2.1.228 fix directly: resolveCherryPickConflict must
// check the git sequencer before trusting herdr's tab-closed signal, not
// after. It scripts TabClose to fail the test if it fires before
// CherryPickInProgress has already been consulted for the conflict pane's
// resolution — the reverse of today's bug, where the tab closed (and the
// child ticket was marked done) before any sequencer check ran at all.
func TestRun_ConflictResolution_CorroboratesSequencerBeforeClosingTab(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()

	inProgress := false
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		inProgress = true
		return &fakeConflictErr{}
	}

	var mu sync.Mutex
	var conflictPaneTab string
	origTabCreate := d.TabCreate
	d.TabCreate = func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error) {
		created, err := origTabCreate(opts)
		if strings.HasPrefix(opts.Label, "conflict-") {
			mu.Lock()
			conflictPaneTab = created.Tab.TabID
			mu.Unlock()
		}
		return created, err
	}

	origAgentPrompt := d.AgentPrompt
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if opts.Text == "/gx-resolving-merge-conflicts" {
			mu.Lock()
			inProgress = false
			mu.Unlock()
		}
		return origAgentPrompt(opts)
	}

	var sequencerCheckedBeforeClose bool
	d.CherryPickInProgress = func(dir string) (bool, error) {
		mu.Lock()
		sequencerCheckedBeforeClose = true
		cur := inProgress
		mu.Unlock()
		return cur, nil
	}
	origTabClose := d.TabClose
	d.TabClose = func(tabID string) error {
		mu.Lock()
		isConflictTab := conflictPaneTab != "" && tabID == conflictPaneTab
		checked := sequencerCheckedBeforeClose
		mu.Unlock()
		if isConflictTab && !checked {
			t.Errorf("conflict-resolution tab %s closed before the sequencer was corroborated", tabID)
		}
		return origTabClose(tabID)
	}

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
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md":                    "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"01a-conflict-resolution.md": "---\nid: \"01a\"\nstatus: claimed\ntype: conflict-resolution\nparent: \"01\"\n---\n# Conflict resolution for 01\n",
	})
	d, _, _ := fakeDeps()

	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-conflict-01", Label: "conflict-01", WorkspaceID: workspaceID}}, nil
	}

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

	var mu sync.Mutex
	var conflictTabCreates int
	origTabCreate := d.TabCreate
	d.TabCreate = func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error) {
		if strings.HasPrefix(opts.Label, "conflict-") {
			mu.Lock()
			conflictTabCreates++
			mu.Unlock()
		}
		return origTabCreate(opts)
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if atomic.LoadInt32(&aborts) != 0 {
		t.Errorf("AbortCherryPick calls = %d, want 0 (must reattach the live resolver, not abort its sequencer state)", aborts)
	}
	if conflictTabCreates != 0 {
		t.Errorf("conflict-labeled TabCreate calls = %d, want 0 (must reuse the live resolver's tab, not fork a second one)", conflictTabCreates)
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

func TestRun_ZeroCommitIteration_MarksNeedsAnswerAndLeavesWorktree(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, removed := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}

	// The zero-commit iteration leaves its ticket needs-answer, so the epic's
	// only ticket is one a human must clear: the run parks on it.
	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})

	if len(*removed) != 0 {
		t.Errorf("removed worktree branches = %v, want the zero-commit iteration's worktree left in place", *removed)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket not marked needs-answer after zero-commit iteration:\n%s", raw)
	}
	if strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket must not be marked done after a zero-commit iteration:\n%s", raw)
	}
}

func TestRun_ZeroCommitIteration_OtherTicketsStillLand(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
	})
	d, _, removed := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		if strings.Contains(dir, "epic-item-01") {
			return 0, nil
		}
		return 1, nil
	}

	// Ticket 01 ends needs-answer, so the run parks on it once 02 has landed.
	runUntilParked(t, RunOptions{
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
	if !strings.Contains(string(raw01), "status: needs-answer") {
		t.Errorf("ticket 01 not marked needs-answer:\n%s", raw01)
	}

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
// waiting instead of the loop marking the ticket needs-answer and abandoning a
// worktree that was about to land a commit.
func TestRun_TransientIdleBlip_DoesNotOrphanCommit(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, removed := fakeDeps()

	var finishCalls int32
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		if !slices.Contains(opts.Until, "done") {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		}
		if atomic.AddInt32(&finishCalls, 1) == 2 {
			// The debounce recheck: the agent went back to work in the
			// meantime, so this "confirm" poll should see it still busy.
			return herdr.Agent{}, errors.New("timed out waiting for agent")
		}
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if got := atomic.LoadInt32(&finishCalls); got < 3 {
		t.Fatalf("AgentWait finish-poll calls = %d, want at least 3 (initial idle, failed confirm, real finish)", got)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket not marked done despite the agent finishing after the transient blip:\n%s", raw)
	}
	if strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket wrongly marked needs-answer from a transient idle blip:\n%s", raw)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the iteration's worktree removed", *removed)
	}
}

// TestRun_CommitLandsDuringNeedsAnswerRecheck_MarksDoneNotNeedsAnswer covers
// finishIteration's own recheck (loop.go): even after waitForFinish's
// confirmFinished settles on "finished", a commit can still land in the
// window before CommitsAhead is checked (e.g. a reattached iteration, which
// skips waitForFinish's debounce). The recheck should catch it instead of
// orphaning the ticket as needs-answer.
func TestRun_CommitLandsDuringNeedsAnswerRecheck_MarksDoneNotNeedsAnswer(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
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
		t.Errorf("ticket not marked done after commit landed on recheck:\n%s", raw)
	}
	if strings.Contains(string(raw), "status: needs-answer") {
		t.Errorf("ticket wrongly marked needs-answer despite commit landing on recheck:\n%s", raw)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the iteration's worktree removed", *removed)
	}
}
