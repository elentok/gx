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

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets/schema"
)

// readyTimer is a Deps.PauseTimer that has already fired: a pause test polls at
// full speed instead of waiting out a real interval.
func readyTimer(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

// assertPauseInterval fails unless dur is the pause poll interval — PauseTimer is
// the pause's own wait, so anything else means the run is waiting on something
// this test did not intend to script.
func assertPauseInterval(t *testing.T, dur time.Duration) {
	t.Helper()
	if dur != pausedPollInterval {
		t.Errorf("PauseTimer duration = %v, want %v", dur, pausedPollInterval)
	}
}

// clearOnPause returns a Deps.PauseTimer replacement for a paused run: the first
// poll rewrites clearPath's status to newStatus, so the pause's next pass sees
// a ticket a "human" cleared while it was waiting. Without it a paused run
// polls forever, which is the point of the feature and the hazard of testing
// it — every pause test needs some scripted clearing hand.
func clearOnPause(t *testing.T, clearPath, newStatus string) (func(time.Duration) <-chan time.Time, *int) {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	return func(dur time.Duration) <-chan time.Time {
		assertPauseInterval(t, dur)
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

// TestRun_StalledTicket_PausesInsteadOfExiting covers ticket 08's core AC: a
// run whose only remaining ticket is paused (answer) neither exits nor errors — it
// pauses, notifies, and carries on once the status clears.
func TestRun_StalledTicket_PausesInsteadOfExiting(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-stuck.md": "---\nid: \"01\"\nstatus: paused\npause_reason: zero-commit\ntype: implement\n---\n# Stuck\n",
	})
	d, prompts, _ := fakeDeps()
	// The pause has no timeout: only the scripted clearing hand ends it.
	pauseTimer, polls := clearOnPause(t, ticketPath(scratchDir, "my-epic", "01-stuck.md"), "open")
	d.PauseTimer = pauseTimer
	sink := &recordingSink{}

	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (a stalled run pauses, it doesn't error)", err)
	}

	if *polls == 0 {
		t.Errorf("run never paused (no pause poll), want it to pause on the paused ticket")
	}
	if len(sink.pausedStalled) != 1 || len(sink.pausedStalled[0]) != 1 || sink.pausedStalled[0][0].Identifier != "01" {
		t.Errorf("EpicPaused calls = %v, want one naming ticket 01", sink.pausedStalled)
	}
	if len(*prompts) != 1 {
		t.Errorf("prompts = %v, want the cleared ticket to be claimed and run once", *prompts)
	}
}

// TestRun_DraftOnlyEpic_Pauses covers the other human-clearable status:
// draft. Unlike paused (answer)/paused (repair), IsPaused matches it off
// the raw frontmatter Status: rather than RenderedStatus, so this proves a
// draft ticket pauses rather than deadlocking.
func TestRun_DraftOnlyEpic_Pauses(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-stub.md": "---\nid: \"01\"\nstatus: draft\ntype: implement\n---\n# Stub\n",
	})
	d, prompts, _ := fakeDeps()
	pauseTimer, polls := clearOnPause(t, ticketPath(scratchDir, "my-epic", "01-stub.md"), "open")
	d.PauseTimer = pauseTimer
	sink := &recordingSink{}

	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (a draft-only epic pauses, it doesn't error)", err)
	}

	if *polls == 0 {
		t.Errorf("run never paused (no pause poll), want it to pause on the draft ticket")
	}
	if len(sink.pausedStalled) != 1 || len(sink.pausedStalled[0]) != 1 || sink.pausedStalled[0][0].Identifier != "01" {
		t.Errorf("EpicPaused calls = %v, want one naming ticket 01", sink.pausedStalled)
	}
	if len(*prompts) != 1 {
		t.Errorf("prompts = %v, want the cleared ticket to be claimed and run once", *prompts)
	}
}

// TestRun_StalledIteration_RegistryClearedAndRelaunched covers the other half
// of ticket 08's registry-clearing claim: not a ticket that starts stalled at
// reattach (see TestRun_StalledTicket_PausesInsteadOfExiting), but one that
// stalls mid-run, whose launched-registry entry must be cleared so a human
// clearing it back to open lets the same run reclaim and finish it. CommitsAhead
// is scripted off whether the human has cleared the ticket yet, not a call
// counter: finishIteration rechecks it once via its own debounce even on the
// very first, still-stalled pass, so a counter would tell the two apart on the
// wrong call.
func TestRun_StalledIteration_RegistryClearedAndRelaunched(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
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
	d.PauseTimer = func(dur time.Duration) <-chan time.Time {
		assertPauseInterval(t, dur)
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

	// The relaunch adopts the paused iteration's still-live session, which has
	// already had its turn, so it is not prompted again.
	if len(*prompts) != 1 {
		t.Errorf("prompts = %v, want 1 (the stalled iteration's only, adopted by the relaunch once the registry entry cleared)", *prompts)
	}
	got := mustParse(t, path)
	if got.Status != schema.StatusDone {
		t.Errorf("final Status = %q, want done", got.Status)
	}
	if len(sink.pausedStalled) != 1 || len(sink.pausedStalled[0]) != 1 || sink.pausedStalled[0][0].Identifier != "01" {
		t.Errorf("EpicPaused calls = %v, want one naming ticket 01", sink.pausedStalled)
	}
}

// TestRun_ClearedRepairWithLiveIteration_ReattachesInsteadOfDoubleLaunching
// covers ticket 09's core claim: iteration ownership decides resume, not the
// ticket's status. Ticket 01's first launch errors out (a git hiccup) before
// ever prompting, paused (repair) with no goroutine left — but its herdr tab
// is scripted to still be live once a human clears it back to open, so the
// run must reattach rather than assume "open" means "never launched" and
// double-launch a second iteration.
func TestRun_ClearedRepairWithLiveIteration_ReattachesInsteadOfDoubleLaunching(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
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

	polls := 0
	d.PauseTimer = func(dur time.Duration) <-chan time.Time {
		assertPauseInterval(t, dur)
		mu.Lock()
		defer mu.Unlock()
		polls++
		if polls == 1 {
			if err := SetStatus(path, "open"); err != nil {
				t.Errorf("SetStatus: %v", err)
			}
			if _, err := fakeRunner(d).Start(agentrunner.StartOptions{Label: "my-epic-iter-01", Epic: "my-epic"}); err != nil {
				t.Errorf("Start: %v", err)
			}
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
	if len(sink.pausedStalled) != 1 || len(sink.pausedStalled[0]) != 1 || sink.pausedStalled[0][0].Identifier != "01" {
		t.Errorf("EpicPaused calls = %v, want one naming ticket 01", sink.pausedStalled)
	}
	want := [4]string{"01", "my-epic-iter-01", "/fake/worktrees/my-epic-item-01", "sess-pane-my-epic-iter-01"}
	if len(sink.reattachedCalls) != 1 || sink.reattachedCalls[0] != want {
		t.Errorf("TicketReattached calls = %v, want exactly one %v", sink.reattachedCalls, want)
	}
	if calls := sink.snapshot(); slices.Contains(calls, "IterationStarted") {
		t.Errorf("calls = %v, want no IterationStarted for a mid-run reattach of a still-live pane", calls)
	}
}

// TestRun_NothingRunnableAndNothingClearable_Deadlocks keeps the genuine
// corruption signal: a blocker naming a ticket that will never resolve has no
// human-clearable ticket to pause on, so it must still error.
func TestRun_NothingRunnableAndNothingClearable_Deadlocks(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-cycle-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\nblocked_by: [\"02\"]\n---\n# A\n",
		"02-cycle-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\nblocked_by: [\"01\"]\n---\n# B\n",
	})
	d, _, _ := fakeDeps()
	sink := &recordingSink{}

	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)
	if err == nil {
		t.Fatalf("Run() error = nil, want a deadlock error for a dependency cycle")
	}
	if len(sink.pausedStalled) != 0 {
		t.Errorf("EpicPaused calls = %v, want none for a deadlocked epic", sink.pausedStalled)
	}
}

// TestRun_StaysPaused_NeverReportsEpicComplete pins the hazard the retired
// test-only pause-poll cap created: exhausting it broke out of the scheduling
// loop into the epic-complete path, letting a test seam fabricate a terminal
// event for a run still waiting on a person. The Run goroutine here pauses
// forever by design — its timer never fires and nobody clears the ticket — so
// it stays blocked in the pause select rather than leaking work.
func TestRun_StaysPaused_NeverReportsEpicComplete(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-stuck.md": "---\nid: \"01\"\nstatus: paused\npause_reason: zero-commit\ntype: implement\n---\n# Stuck\n",
	})
	d, _, _ := fakeDeps()
	polled := make(chan struct{})
	never := make(chan time.Time)
	var once sync.Once
	d.PauseTimer = func(dur time.Duration) <-chan time.Time {
		assertPauseInterval(t, dur)
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
		t.Fatalf("Run() returned %v before pausing, want it to pause on the paused ticket", err)
	case <-time.After(5 * time.Second):
		t.Fatal("run never paused")
	}
	select {
	case err := <-done:
		t.Fatalf("Run() returned %v while its only ticket was still paused, want it to stay paused", err)
	case <-time.After(100 * time.Millisecond):
	}

	for _, call := range sink.snapshot() {
		if call == "EpicComplete" {
			t.Fatalf("sink calls = %v, want no EpicComplete for a still-paused epic", sink.snapshot())
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not stop after cancellation")
	}
}

// TestRun_EpicPaused_ZeroCommitPauseReportsNotReattachable covers the third
// of the ticket's three call sites: EpicPaused's Reattachable flag (surfaced
// in Telegram notifications) must not claim a zero-commit pause is
// reattachable when gx will never auto-reattach it. Before this predicate
// existed, Reattachable was liveness-only, so a live-but-idle zero-commit
// pause (nothing merged) would have wrongly reported true — the exact
// mismatch this ticket closes between the notification and resumeAnswered's
// own refusal to clear the same ticket.
func TestRun_EpicPaused_ZeroCommitPauseReportsNotReattachable(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) { return 0, nil }
	sink := &recordingSink{}

	runUntilPaused(t, RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)

	if len(sink.pausedStalled) == 0 {
		t.Fatalf("EpicPaused calls = %v, want at least one", sink.pausedStalled)
	}
	last := sink.pausedStalled[len(sink.pausedStalled)-1]
	if len(last) != 1 || last[0].Identifier != "01" {
		t.Fatalf("EpicPaused stalled = %v, want exactly ticket 01", last)
	}
	if last[0].Reattachable {
		t.Errorf("zero-commit ticket 01 Reattachable = true, want false (live pane but no new commits)")
	}
}

// TestGate_WakePaused_ShortensPauseWait proves the cosmetic-wake mechanism
// loop.go's pause branch relies on: WakePaused cuts a pause wait short instead
// of it running out a long pause-poll-interval wait.
func TestGate_WakePaused_ShortensPauseWait(t *testing.T) {
	t.Parallel()
	gate := NewGate()
	waiting := make(chan struct{})
	woke := make(chan struct{})

	go func() {
		timer := time.After(time.Hour)
		pauseWake := gate.PauseWake()
		close(waiting)
		select {
		case <-timer:
		case <-pauseWake:
		}
		close(woke)
	}()

	<-waiting
	gate.WakePaused()

	select {
	case <-woke:
	case <-time.After(5 * time.Second):
		t.Fatal("WakePaused() did not interrupt an in-progress pause wait")
	}
}

// deadlockedKinds lists the kind of every deadlocked event in epicName's run log.
func deadlockedKinds(t *testing.T, scratchDir, epicName string) []string {
	t.Helper()
	evs, _, err := ReadEvents(scratchDir, epicName)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	var kinds []string
	for _, ev := range evs {
		if ev.Type == string(events.Deadlocked) {
			kinds = append(kinds, ev.Kind)
		}
	}
	return kinds
}

// TestRun_Pause_EmitsDeadlockedOncePerEntry is the all-paused half of the
// deadlocked seam: entering the pause emits one event, the repeated pause polls
// do not, and the epic can enter deadlock again after it left.
func TestRun_Pause_EmitsDeadlockedOncePerEntry(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-stuck.md": "---\nid: \"01\"\nstatus: paused\npause_reason: iteration-error\ntype: implement\n---\n# Stuck\n",
	})
	d, _, _ := fakeDeps()
	path := ticketPath(scratchDir, "my-epic", "01-stuck.md")
	polls := 0
	d.PauseTimer = func(dur time.Duration) <-chan time.Time {
		assertPauseInterval(t, dur)
		polls++
		if polls == 3 {
			if got := deadlockedKinds(t, scratchDir, "my-epic"); !slices.Equal(got, []string{"all-paused"}) {
				t.Errorf("deadlocked events after repeated polls = %v, want one all-paused", got)
			}
			if err := SetStatus(path, "open"); err != nil {
				t.Errorf("SetStatus: %v", err)
			}
		}
		return readyTimer(dur)
	}

	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

// TestRun_Pause_AnswerOnlyEmitsWaitingForAnswer: a pause that only a person's
// answer clears is a normal wait, so it logs waiting-for-answer, not deadlocked.
func TestRun_Pause_AnswerOnlyEmitsWaitingForAnswer(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-asks.md": "---\nid: \"01\"\nstatus: paused\npause_reason: zero-commit\ntype: implement\n---\n# Asks\n",
	})
	d, _, _ := fakeDeps()
	path := ticketPath(scratchDir, "my-epic", "01-asks.md")
	polls := 0
	d.PauseTimer = func(dur time.Duration) <-chan time.Time {
		polls++
		if polls == 3 {
			if got := deadlockedKinds(t, scratchDir, "my-epic"); len(got) != 0 {
				t.Errorf("deadlocked events = %v, want none", got)
			}
			evs, _, err := ReadEvents(scratchDir, "my-epic")
			if err != nil {
				t.Fatalf("ReadEvents: %v", err)
			}
			waits := 0
			for _, ev := range evs {
				if ev.Type == string(events.WaitingForAnswer) {
					waits++
				}
			}
			if waits != 1 {
				t.Errorf("waiting-for-answer events = %d, want 1", waits)
			}
			if err := SetStatus(path, "open"); err != nil {
				t.Errorf("SetStatus: %v", err)
			}
		}
		return readyTimer(dur)
	}

	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

// TestRun_BlockedCycle_EmitsDeadlocked is the other kind: nothing runnable and
// nothing a person could clear.
func TestRun_BlockedCycle_EmitsDeadlocked(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-cycle-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\nblocked_by: [\"02\"]\n---\n# A\n",
		"02-cycle-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\nblocked_by: [\"01\"]\n---\n# B\n",
	})
	d, _, _ := fakeDeps()

	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{}); err == nil {
		t.Fatal("Run() error = nil, want a deadlock error")
	}
	if got := deadlockedKinds(t, scratchDir, "my-epic"); !slices.Equal(got, []string{"blocked-cycle"}) {
		t.Errorf("deadlocked events = %v, want one blocked-cycle", got)
	}
}
