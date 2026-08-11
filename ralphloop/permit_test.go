package ralphloop

import (
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
)

// fakePermit is a minimal test Permit: Acquire/Release just call the
// (optional) hook, so each test wires only the behavior it needs.
type fakePermit struct {
	acquire func()
	release func()
}

func (p *fakePermit) Acquire() {
	if p.acquire != nil {
		p.acquire()
	}
}

func (p *fakePermit) Release() {
	if p.release != nil {
		p.release()
	}
}

// TestRun_NilPermit_BehavesUnrestricted is regression coverage for
// RunOptions.Permit left unset: every existing caller/test keeps today's
// unrestricted behavior (mirrors TestRun_StalledTicket_ParksInsteadOfExiting's
// setup without duplicating its full assertion set).
func TestRun_NilPermit_BehavesUnrestricted(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-stuck.md": "---\nid: \"01\"\nstatus: needs-answer\ntype: task\n---\n# Stuck\n",
	})
	d, prompts, _ := fakeDeps()
	parkTimer, polls := clearOnPark(t, ticketPath(scratchDir, "my-epic", "01-stuck.md"), "open")
	d.ParkTimer = parkTimer

	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &recordingSink{})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (no Permit set, unrestricted behavior)", err)
	}
	if *polls == 0 {
		t.Errorf("run never parked (no park poll), want it to park on the needs-answer ticket")
	}
	if len(*prompts) != 1 {
		t.Errorf("prompts = %v, want the cleared ticket claimed and run once", *prompts)
	}
}

// TestRun_Permit_AcquiredOnClaimReleasedOnPark proves the park transition
// releases the slot: a fake Permit fails the test outright if Acquire is
// called a second time without an intervening Release, and the run must
// Acquire at least twice — once before the initial claim attempt finds
// nothing runnable and parks, once again after a human clears the ticket.
func TestRun_Permit_AcquiredOnClaimReleasedOnPark(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-stuck.md": "---\nid: \"01\"\nstatus: needs-answer\ntype: task\n---\n# Stuck\n",
	})
	d, prompts, _ := fakeDeps()
	parkTimer, polls := clearOnPark(t, ticketPath(scratchDir, "my-epic", "01-stuck.md"), "open")
	d.ParkTimer = parkTimer

	var mu sync.Mutex
	held := false
	acquireCalls := 0
	releaseCalls := 0
	permit := &fakePermit{
		acquire: func() {
			mu.Lock()
			defer mu.Unlock()
			if held {
				t.Errorf("Acquire called while already held (no intervening Release)")
			}
			held = true
			acquireCalls++
		},
		release: func() {
			mu.Lock()
			defer mu.Unlock()
			if !held {
				t.Errorf("Release called while not held")
			}
			held = false
			releaseCalls++
		},
	}

	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo", Permit: permit}, d, &recordingSink{})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (a stalled run parks, it doesn't error)", err)
	}
	if *polls == 0 {
		t.Errorf("run never parked (no park poll)")
	}
	if len(*prompts) != 1 {
		t.Errorf("prompts = %v, want the cleared ticket claimed and run once", *prompts)
	}

	mu.Lock()
	defer mu.Unlock()
	if acquireCalls < 2 {
		t.Errorf("Acquire calls = %d, want at least 2 (initial park attempt, then the relaunch after clearing)", acquireCalls)
	}
	if releaseCalls == 0 {
		t.Errorf("Release calls = %d, want at least 1 (released at the park)", releaseCalls)
	}
	if held {
		t.Errorf("permit left held after Run returned, want released (defer releasePermit)")
	}
}

// TestRun_Permit_BlocksClaimUntilAcquireReturns proves Run actually blocks on
// a slow Permit.Acquire rather than claiming anyway: AgentPrompt must never
// fire before the fake's Acquire call returns.
func TestRun_Permit_BlocksClaimUntilAcquireReturns(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()

	unblock := make(chan struct{})
	var mu sync.Mutex
	acquireReturned := false
	permit := &fakePermit{
		acquire: func() {
			<-unblock
			mu.Lock()
			acquireReturned = true
			mu.Unlock()
		},
	}

	origAgentPrompt := d.AgentPrompt
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		mu.Lock()
		ready := acquireReturned
		mu.Unlock()
		if !ready {
			t.Errorf("AgentPrompt called before Permit.Acquire returned")
		}
		return origAgentPrompt(opts)
	}

	runErr := make(chan error, 1)
	go func() {
		runErr <- Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo", Permit: permit}, d, &recordingSink{})
	}()

	// Give Run a moment to reach and block on Acquire before releasing it.
	time.Sleep(50 * time.Millisecond)
	close(unblock)

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Run to finish after unblocking Acquire")
	}
}

// TestRun_EpicStarted_FiresAfterPermitAcquireReturns proves EpicStarted
// waits for a slow Permit.Acquire to return before firing: an epic waiting
// for a slot hasn't started yet, so the start message must not report a
// start that hasn't happened.
func TestRun_EpicStarted_FiresAfterPermitAcquireReturns(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()

	unblock := make(chan struct{})
	permit := &fakePermit{
		acquire: func() { <-unblock },
	}

	sink := &recordingSink{}
	runErr := make(chan error, 1)
	go func() {
		runErr <- Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo", Permit: permit}, d, sink)
	}()

	// Give Run a moment to reach and block on Acquire before releasing it.
	time.Sleep(50 * time.Millisecond)
	if got := sink.snapshot(); len(got) != 0 {
		t.Errorf("events = %v, want none while still blocked on Permit.Acquire", got)
	}
	close(unblock)

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Run to finish after unblocking Acquire")
	}

	if got := sink.snapshot(); len(got) == 0 || got[0] != "EpicStarted" {
		t.Errorf("events = %v, want EpicStarted first", got)
	}
}

// TestRun_Permit_AcquiredBeforeReattachLaunch confirms Acquire is called
// before the reattach-launch block runs, not just before a fresh claim: a
// ticket that reconciles as already-running (a live tab found by TabList)
// must not be launched until the permit is held.
func TestRun_Permit_AcquiredBeforeReattachLaunch(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: claimed\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{Label: iterLabel("my-epic", "01"), WorkspaceID: workspaceID, TabID: "tab-01"}}, nil
	}

	var mu sync.Mutex
	acquired := false
	permit := &fakePermit{
		acquire: func() {
			mu.Lock()
			acquired = true
			mu.Unlock()
		},
	}

	origAgentWait := d.AgentWait
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		mu.Lock()
		ready := acquired
		mu.Unlock()
		if !ready {
			t.Errorf("AgentWait (reattach launch) called before Permit.Acquire")
		}
		return origAgentWait(opts)
	}

	// The reattached iteration ends stalled, so the run parks on it.
	runUntilParked(t, RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo", Permit: permit}, d, &recordingSink{})

	mu.Lock()
	defer mu.Unlock()
	if !acquired {
		t.Errorf("Permit.Acquire was never called")
	}
}
