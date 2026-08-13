package ralphloop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets/schema"
)

// readyTimer is a Deps.ParkTimer that has already fired: a park test polls at
// full speed instead of waiting out a real interval.
func readyTimer(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

// assertParkInterval fails unless dur is the park poll interval — ParkTimer is
// the park's own wait, so anything else means the run is waiting on something
// this test did not intend to script.
func assertParkInterval(t *testing.T, dur time.Duration) {
	t.Helper()
	if dur != parkPollInterval {
		t.Errorf("ParkTimer duration = %v, want %v", dur, parkPollInterval)
	}
}

// clearOnPark returns a Deps.ParkTimer replacement for a parked run: the first
// poll rewrites clearPath's status to newStatus, so the park's next pass sees
// a ticket a "human" cleared while it was waiting. Without it a parked run
// polls forever, which is the point of the feature and the hazard of testing
// it — every park test needs some scripted clearing hand.
func clearOnPark(t *testing.T, clearPath, newStatus string) (func(time.Duration) <-chan time.Time, *int) {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	return func(dur time.Duration) <-chan time.Time {
		assertParkInterval(t, dur)
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 1 {
			raw, err := os.ReadFile(clearPath)
			if err != nil {
				t.Errorf("ReadFile %s: %v", clearPath, err)
			} else if err := SetStatus(clearPath, newStatus); err != nil {
				t.Errorf("SetStatus %s: %v (was %q)", clearPath, err, raw)
			}
		}
		return readyTimer(dur)
	}, &calls
}

func ticketPath(scratchDir, epicName, file string) string {
	return filepath.Join(scratchDir, epicName, "issues", file)
}

// TestRun_StalledTicket_ParksInsteadOfExiting covers ticket 08's core AC: a
// run whose only remaining ticket is needs-answer neither exits nor errors — it
// parks, notifies, and carries on once the status clears.
func TestRun_StalledTicket_ParksInsteadOfExiting(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-stuck.md": "---\nid: \"01\"\nstatus: needs-answer\ntype: task\n---\n# Stuck\n",
	})
	d, prompts, _ := fakeDeps()
	// The park has no timeout: only the scripted clearing hand ends it.
	parkTimer, polls := clearOnPark(t, ticketPath(scratchDir, "my-epic", "01-stuck.md"), "open")
	d.ParkTimer = parkTimer
	sink := &recordingSink{}

	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (a stalled run parks, it doesn't error)", err)
	}

	if *polls == 0 {
		t.Errorf("run never parked (no park poll), want it to park on the needs-answer ticket")
	}
	if len(sink.parkedStalled) != 1 || len(sink.parkedStalled[0]) != 1 || sink.parkedStalled[0][0].Identifier != "01" {
		t.Errorf("EpicParked calls = %v, want one naming ticket 01", sink.parkedStalled)
	}
	if len(*prompts) != 1 {
		t.Errorf("prompts = %v, want the cleared ticket to be claimed and run once", *prompts)
	}
}

// TestRun_DraftOnlyEpic_Parks covers the other human-clearable status:
// draft. Unlike needs-answer/needs-repair, isParked matches it off
// the raw frontmatter Status: rather than RenderedStatus, so this proves a
// draft ticket parks rather than deadlocking.
func TestRun_DraftOnlyEpic_Parks(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-stub.md": "---\nid: \"01\"\nstatus: draft\ntype: task\n---\n# Stub\n",
	})
	d, prompts, _ := fakeDeps()
	parkTimer, polls := clearOnPark(t, ticketPath(scratchDir, "my-epic", "01-stub.md"), "open")
	d.ParkTimer = parkTimer
	sink := &recordingSink{}

	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (a draft-only epic parks, it doesn't error)", err)
	}

	if *polls == 0 {
		t.Errorf("run never parked (no park poll), want it to park on the draft ticket")
	}
	if len(sink.parkedStalled) != 1 || len(sink.parkedStalled[0]) != 1 || sink.parkedStalled[0][0].Identifier != "01" {
		t.Errorf("EpicParked calls = %v, want one naming ticket 01", sink.parkedStalled)
	}
	if len(*prompts) != 1 {
		t.Errorf("prompts = %v, want the cleared ticket to be claimed and run once", *prompts)
	}
}

// TestRun_StalledIteration_RegistryClearedAndRelaunched covers the other half
// of ticket 08's registry-clearing claim: not a ticket that starts stalled at
// reattach (see TestRun_StalledTicket_ParksInsteadOfExiting), but one that
// stalls mid-run, whose launched-registry entry must be cleared so a human
// clearing it back to open lets the same run reclaim and finish it. CommitsAhead
// is scripted off whether the human has cleared the ticket yet, not a call
// counter: finishIteration rechecks it once via its own debounce even on the
// very first, still-stalled pass, so a counter would tell the two apart on the
// wrong call.
func TestRun_StalledIteration_RegistryClearedAndRelaunched(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	path := ticketPath(scratchDir, "my-epic", "01-a.md")
	d, prompts, _ := fakeDeps()

	var mu sync.Mutex
	humanCleared := false
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		if humanCleared {
			return 1, nil
		}
		return 0, nil
	}
	polls := 0
	d.ParkTimer = func(dur time.Duration) <-chan time.Time {
		assertParkInterval(t, dur)
		mu.Lock()
		defer mu.Unlock()
		polls++
		if polls == 1 {
			if err := SetStatus(path, "open"); err != nil {
				t.Errorf("SetStatus: %v", err)
			}
			humanCleared = true
		}
		return readyTimer(dur)
	}
	sink := &recordingSink{}

	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	if len(*prompts) != 2 {
		t.Errorf("prompts = %v, want 2 (the stalled iteration, then a relaunch once the registry entry cleared)", *prompts)
	}
	got := mustParse(t, path)
	if got.Status != schema.StatusDone {
		t.Errorf("final Status = %q, want done", got.Status)
	}
	if len(sink.parkedStalled) != 1 || len(sink.parkedStalled[0]) != 1 || sink.parkedStalled[0][0].Identifier != "01" {
		t.Errorf("EpicParked calls = %v, want one naming ticket 01", sink.parkedStalled)
	}
}

// TestRun_ClearedNeedsRepairWithLiveIteration_ReattachesInsteadOfDoubleLaunching
// covers ticket 09's core claim: iteration ownership decides resume, not the
// ticket's status. Ticket 01's first launch errors out (a git hiccup) before
// ever prompting, needs-repair with no goroutine left — but its herdr tab
// is scripted to still be live once a human clears it back to open, so the
// run must reattach rather than assume "open" means "never launched" and
// double-launch a second iteration.
func TestRun_ClearedNeedsRepairWithLiveIteration_ReattachesInsteadOfDoubleLaunching(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	path := ticketPath(scratchDir, "my-epic", "01-a.md")
	d, prompts, removed := fakeDeps()

	var mu sync.Mutex
	addWorktreeCalls := 0
	origAddWorktree := d.AddWorktree
	failedOnce := false
	d.AddWorktree = func(repoDir, wtPath, branch, base string) error {
		if !strings.Contains(wtPath, "my-epic-item-01") {
			return origAddWorktree(repoDir, wtPath, branch, base)
		}
		mu.Lock()
		defer mu.Unlock()
		addWorktreeCalls++
		if !failedOnce {
			failedOnce = true
			return errors.New("simulated git hiccup")
		}
		return origAddWorktree(repoDir, wtPath, branch, base)
	}

	cleared := false
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		mu.Lock()
		defer mu.Unlock()
		if !cleared {
			return nil, nil
		}
		return []herdr.Tab{{TabID: "tab-my-epic-iter-01", Label: "my-epic-iter-01", WorkspaceID: workspaceID}}, nil
	}

	polls := 0
	d.ParkTimer = func(dur time.Duration) <-chan time.Time {
		assertParkInterval(t, dur)
		mu.Lock()
		defer mu.Unlock()
		polls++
		if polls == 1 {
			if err := SetStatus(path, "open"); err != nil {
				t.Errorf("SetStatus: %v", err)
			}
			cleared = true
		}
		return readyTimer(dur)
	}
	sink := &recordingSink{}

	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	if addWorktreeCalls != 1 {
		t.Errorf("AddWorktree calls = %d, want 1 (a successful reattach reuses the live iteration instead of creating a second one)", addWorktreeCalls)
	}
	if len(*prompts) != 0 {
		t.Errorf("prompts = %v, want none (the original launch errored before prompting, and reattach never replays it)", *prompts)
	}
	got := mustParse(t, path)
	if got.Status != schema.StatusDone {
		t.Errorf("final Status = %q, want done", got.Status)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want exactly one removal", *removed)
	}
	if len(sink.parkedStalled) != 1 || len(sink.parkedStalled[0]) != 1 || sink.parkedStalled[0][0].Identifier != "01" {
		t.Errorf("EpicParked calls = %v, want one naming ticket 01", sink.parkedStalled)
	}
	want := [4]string{"01", "my-epic-iter-01", "/fake/worktrees/my-epic-item-01", "session-my-epic-iter-01"}
	if len(sink.reattachedCalls) != 1 || sink.reattachedCalls[0] != want {
		t.Errorf("TicketReattached calls = %v, want exactly one %v", sink.reattachedCalls, want)
	}
	if calls := sink.snapshot(); slices.Contains(calls, "IterationStarted") {
		t.Errorf("calls = %v, want no IterationStarted for a mid-run reattach of a still-live pane", calls)
	}
}

// TestRun_NothingRunnableAndNothingClearable_Deadlocks keeps the genuine
// corruption signal: a blocker naming a ticket that will never resolve has no
// human-clearable ticket to park on, so it must still error.
func TestRun_NothingRunnableAndNothingClearable_Deadlocks(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-cycle-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\nblocked_by: [\"02\"]\n---\n# A\n",
		"02-cycle-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# B\n",
	})
	d, _, _ := fakeDeps()
	sink := &recordingSink{}

	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)
	if err == nil {
		t.Fatalf("Run() error = nil, want a deadlock error for a dependency cycle")
	}
	if len(sink.parkedStalled) != 0 {
		t.Errorf("EpicParked calls = %v, want none for a deadlocked epic", sink.parkedStalled)
	}
}

// TestRun_StaysParked_NeverReportsEpicComplete pins the hazard the retired
// test-only park-poll cap created: exhausting it broke out of the scheduling
// loop into the epic-complete path, letting a test seam fabricate a terminal
// event for a run still waiting on a person. The Run goroutine here parks
// forever by design — its timer never fires and nobody clears the ticket — so
// it stays blocked in the park select rather than leaking work.
func TestRun_StaysParked_NeverReportsEpicComplete(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-stuck.md": "---\nid: \"01\"\nstatus: needs-answer\ntype: task\n---\n# Stuck\n",
	})
	d, _, _ := fakeDeps()
	polled := make(chan struct{})
	never := make(chan time.Time)
	var once sync.Once
	d.ParkTimer = func(dur time.Duration) <-chan time.Time {
		assertParkInterval(t, dur)
		once.Do(func() { close(polled) })
		return never
	}
	sink := &recordingSink{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	go func() {
		done <- Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo", Ctx: ctx}, d, sink)
	}()

	select {
	case <-polled:
	case err := <-done:
		t.Fatalf("Run() returned %v before parking, want it to park on the needs-answer ticket", err)
	case <-time.After(5 * time.Second):
		t.Fatal("run never parked")
	}
	select {
	case err := <-done:
		t.Fatalf("Run() returned %v while its only ticket was still needs-answer, want it to stay parked", err)
	case <-time.After(100 * time.Millisecond):
	}

	for _, call := range sink.snapshot() {
		if call == "EpicComplete" {
			t.Fatalf("sink calls = %v, want no EpicComplete for a still-parked epic", sink.snapshot())
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not stop after cancellation")
	}
}

// TestRun_EpicParked_ZeroCommitParkReportsNotReattachable covers the third
// of the ticket's three call sites: EpicParked's Reattachable flag (surfaced
// in Telegram notifications) must not claim a zero-commit park is
// reattachable when gx will never auto-reattach it. Before this predicate
// existed, Reattachable was liveness-only, so a live-but-idle zero-commit
// park (nothing landed) would have wrongly reported true — the exact
// mismatch this ticket closes between the notification and unparkAnswered's
// own refusal to clear the same ticket.
func TestRun_EpicParked_ZeroCommitParkReportsNotReattachable(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) { return 0, nil }
	sink := &recordingSink{}

	runUntilParked(t, RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)

	if len(sink.parkedStalled) == 0 {
		t.Fatalf("EpicParked calls = %v, want at least one", sink.parkedStalled)
	}
	last := sink.parkedStalled[len(sink.parkedStalled)-1]
	if len(last) != 1 || last[0].Identifier != "01" {
		t.Fatalf("EpicParked stalled = %v, want exactly ticket 01", last)
	}
	if last[0].Reattachable {
		t.Errorf("zero-commit ticket 01 Reattachable = true, want false (live pane but no new commits)")
	}
}

// TestGate_WakeParked_ShortensParkWait proves the cosmetic-wake mechanism
// loop.go's park branch relies on: WakeParked cuts a park wait short instead
// of it running out a long parkPollInterval-equivalent wait.
func TestGate_WakeParked_ShortensParkWait(t *testing.T) {
	t.Parallel()
	gate := NewGate()
	waiting := make(chan struct{})
	woke := make(chan struct{})

	go func() {
		timer := time.After(time.Hour)
		parkWake := gate.ParkWake()
		close(waiting)
		select {
		case <-timer:
		case <-parkWake:
		}
		close(woke)
	}()

	<-waiting
	gate.WakeParked()

	select {
	case <-woke:
	case <-time.After(5 * time.Second):
		t.Fatal("WakeParked() did not interrupt an in-progress park wait")
	}
}
