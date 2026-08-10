package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/ralphloop"
)

// writeParkResumeEpic builds a fixture epic with a single needs-info ticket
// at <scratchDir>/<epicName>/issues/01-stuck.md, so ralphloop.Run parks on it
// immediately — mirroring ralphloop's own unexported writeEpic test helper
// (ralphloop/loop_test.go), which can't be imported from this package.
func writeParkResumeEpic(t *testing.T, epicName string) (scratchDir, ticketPath string) {
	t.Helper()
	scratchDir = t.TempDir()
	issuesDir := filepath.Join(scratchDir, epicName, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	ticketPath = filepath.Join(issuesDir, "01-stuck.md")
	content := "---\nid: \"01\"\nstatus: needs-info\ntype: task\n---\n# Stuck\n"
	if err := os.WriteFile(ticketPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return scratchDir, ticketPath
}

// fakeParkResumeDeps returns a ralphloop.Deps wired to trivial in-memory
// fakes safe for concurrent use — a local copy of ralphloop's own unexported
// fakeDeps() (ralphloop/loop_test.go), copied because that helper can't be
// imported from this package.
func fakeParkResumeDeps() ralphloop.Deps {
	return ralphloop.Deps{
		AgentGet: func(target string) (herdr.Agent, error) {
			return herdr.Agent{PaneID: "pane-" + target, WorkspaceID: "ws1", TabID: "tab-" + target, AgentStatus: "working", AgentSession: "session-" + target}, nil
		},
		VerifyCodexSession:    func(cwd, sessionID string) (bool, error) { return true, nil },
		FindOrCreateWorkspace: func(label, cwd string) (string, error) { return "ws1", nil },
		WorktreeDir:           func(repoDir string) (string, error) { return "/fake/worktrees", nil },
		AddWorktree:           func(repoDir, path, branch, base string) error { return nil },
		RemoveWorktree:        func(repoDir, path string, force bool) error { return nil },
		DeleteBranch:          func(repoDir, branch string) error { return nil },
		TabCreate: func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error) {
			return herdr.CreatedTab{
				Tab:        herdr.Tab{TabID: "tab-" + opts.Label, Label: opts.Label, WorkspaceID: opts.WorkspaceID},
				RootPaneID: "pane-" + opts.Label,
			}, nil
		},
		TabClose: func(tabID string) error { return nil },
		TabList:  func(workspaceID string) ([]herdr.Tab, error) { return nil, nil },
		AgentStart: func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle"}, nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentRead:            func(target string, opts herdr.AgentReadOptions) (string, error) { return "compaction complete", nil },
		RevParse:             func(dir, ref string) (string, error) { return "deadbeef", nil },
		MergeBase:            func(dir, refA, refB string) (string, error) { return "deadbeef", nil },
		CommitsAhead:         func(dir, fromExclusive, toRef string) (int, error) { return 1, nil },
		CherryPickRange:      func(dir, fromExclusive, toInclusive string) error { return nil },
		CherryPickInProgress: func(dir string) (bool, error) { return false, nil },
		AbortCherryPick:      func(dir string) error { return nil },
		IsAncestor:           func(dir, ancestor, descendant string) (bool, error) { return true, nil },
		PatchesApplied:       func(dir, upstream, base, branch string) (bool, error) { return false, nil },
		AppendTrailers:       func(dir string, trailers ...git.Trailer) error { return nil },
		WorktreeExists:       func(path string) (bool, error) { return true, nil },
		InstallDeps:          func(path string) (string, error) { return "", nil },
		AgentSendKeys:        func(target string, keys ...string) error { return nil },
		ReadOccupancy:        func(cwd, sessionID string) (int, bool, error) { return 0, false, nil },
		Sleep:                func(time.Duration) {},
		Now:                  time.Now,
		ParkTimer:            readyParkTimer,
	}
}

// readyParkTimer is a ralphloop.Deps.ParkTimer that has already fired, so a
// park in this test polls at full speed instead of on the wall clock.
func readyParkTimer(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

// clearStatusOnFirstPark returns a Deps.ParkTimer replacement that rewrites
// ticketPath's status to newStatus on the first park poll — ParkTimer is only
// consulted by the loop's park wait (ralphloop/loop.go), so any call here means
// the run just parked. A park has no timeout, so this scripted clearing hand is
// the only thing that ends it.
func clearStatusOnFirstPark(t *testing.T, ticketPath, newStatus string) func(time.Duration) <-chan time.Time {
	t.Helper()
	var once sync.Once
	return func(dur time.Duration) <-chan time.Time {
		once.Do(func() {
			if err := ralphloop.SetStatus(ticketPath, newStatus); err != nil {
				t.Errorf("SetStatus %s: %v", ticketPath, err)
			}
		})
		return readyParkTimer(dur)
	}
}

// permitProbe wraps a real ralphloop.Permit (here, a *loopRegistry) so the
// test can observe Acquire/Release without reaching into loopRegistry's own
// internals. held/maxHeld are shared (by pointer) across every permitProbe
// wrapping the same registry, so maxHeld reflects the true total number of
// concurrently-held permits across every epic, not just this one adapter's
// own — a per-adapter count would trivially stay <= 1 even if the registry's
// cap were broken, since a single epic never acquires twice at once. It also
// exposes hooks fired just before/after each call to the real Acquire, for
// tests that need to prove a call genuinely blocked rather than just
// happening to run second.
type permitProbe struct {
	real ralphloop.Permit

	held, maxHeld *int32

	onAcquireStart func() // called just before the real Acquire()
	onAcquireDone  func() // called just after the real Acquire() returns
}

func (p *permitProbe) Acquire() {
	if p.onAcquireStart != nil {
		p.onAcquireStart()
	}
	p.real.Acquire()
	held := atomic.AddInt32(p.held, 1)
	for {
		prevMax := atomic.LoadInt32(p.maxHeld)
		if held <= prevMax || atomic.CompareAndSwapInt32(p.maxHeld, prevMax, held) {
			break
		}
	}
	if p.onAcquireDone != nil {
		p.onAcquireDone()
	}
}

func (p *permitProbe) Release() {
	atomic.AddInt32(p.held, -1)
	p.real.Release()
}

// drainChannelEventSink discards every event a ChannelEventSink produces, so
// ralphloop.Run (whose live-event sends block once the sink's internal
// buffer fills) is never starved of a reader.
func drainChannelEventSink(sink *ralphloop.ChannelEventSink) {
	go func() {
		for range sink.Events() {
		}
	}()
}

// TestConcurrentParkResume_TwoEpicsAgainstCapOfOne is ticket 10's integration
// seam: ralphloop.Run plus the real loopRegistry together, proving the cap
// only holds because Run actually acquires a permit before claiming, not
// just because the registry-level unit tests say Acquire/Release behave.
// Two epics, each starting on a parked (needs-info) ticket, resume
// concurrently against a cap of one; the test proves the second epic's
// second Acquire() call — the one guarding its real claimed-ticket work, as
// opposed to the harmless nothing-claimable pass every park poll also
// acquires and immediately releases around — is genuinely blocked while the
// first epic holds the permit, and that neither epic ever observes more
// than one permit held at once.
func TestConcurrentParkResume_TwoEpicsAgainstCapOfOne(t *testing.T) {
	scratch1, ticket1 := writeParkResumeEpic(t, "epic-one")
	scratch2, ticket2 := writeParkResumeEpic(t, "epic-two")

	registry := newLoopRegistry(1)

	// epic1Running closes the first time epic one's claimed ticket actually
	// starts running its iteration (AgentWait is only reached once a ticket
	// has been claimed and launched) — proof epic one currently holds the
	// sole permit. epic1Proceed then holds that iteration open until the
	// test has confirmed epic two's real claim is blocked on it.
	epic1Running := make(chan struct{})
	epic1Proceed := make(chan struct{})
	var epic1RunningOnce sync.Once

	deps1 := fakeParkResumeDeps()
	deps1.ParkTimer = clearStatusOnFirstPark(t, ticket1, "open")
	deps1.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		epic1RunningOnce.Do(func() { close(epic1Running) })
		<-epic1Proceed
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}
	var sharedHeld, sharedMaxHeld int32
	permit1 := &permitProbe{real: registry, held: &sharedHeld, maxHeld: &sharedMaxHeld}

	// epic2AcquireStarted/-Done track only epic two's *second* Acquire call —
	// the one guarding its real claimed-ticket work — via acquireCount; its
	// first Acquire (the nothing-claimable pass while its ticket is still
	// needs-info) is uninteresting and may complete at any time.
	var epic2AcquireCount atomic.Int32
	epic2SecondAcquireStarted := make(chan struct{})
	epic2SecondAcquireDone := make(chan struct{})

	deps2 := fakeParkResumeDeps()
	// epic two's clearing hand waits for epic1Running first, so epic two's
	// real reclaim attempt (its second Acquire) always happens after epic
	// one already holds the sole permit — otherwise the two epics' initial
	// quick nothing-claimable/clear/reclaim cycles could race, and epic two
	// might legitimately acquire first, which would make the "genuinely
	// blocked" assertion below flaky rather than a real proof.
	var epic2ClearOnce sync.Once
	deps2.ParkTimer = func(dur time.Duration) <-chan time.Time {
		epic2ClearOnce.Do(func() {
			<-epic1Running
			if err := ralphloop.SetStatus(ticket2, "open"); err != nil {
				t.Errorf("SetStatus %s: %v", ticket2, err)
			}
		})
		return readyParkTimer(dur)
	}
	permit2 := &permitProbe{
		real: registry, held: &sharedHeld, maxHeld: &sharedMaxHeld,
		onAcquireStart: func() {
			if epic2AcquireCount.Add(1) == 2 {
				close(epic2SecondAcquireStarted)
			}
		},
		onAcquireDone: func() {
			if epic2AcquireCount.Load() == 2 {
				select {
				case <-epic2SecondAcquireDone:
				default:
					close(epic2SecondAcquireDone)
				}
			}
		},
	}

	var wg sync.WaitGroup
	var err1, err2 error

	sink1 := ralphloop.NewChannelEventSink()
	drainChannelEventSink(sink1)
	wg.Go(func() {
		err1 = ralphloop.Run(ralphloop.RunOptions{
			EpicName: "epic-one", Skill: "implement", ScratchDir: scratch1, RepoDir: "/fake/repo-one", Permit: permit1,
		}, deps1, sink1)
	})

	sink2 := ralphloop.NewChannelEventSink()
	drainChannelEventSink(sink2)
	wg.Go(func() {
		err2 = ralphloop.Run(ralphloop.RunOptions{
			EpicName: "epic-two", Skill: "implement", ScratchDir: scratch2, RepoDir: "/fake/repo-two", Permit: permit2,
		}, deps2, sink2)
	})

	// Wait for epic one to actually be running its claimed ticket — it now
	// holds the registry's sole permit, and will keep holding it until the
	// test closes epic1Proceed below.
	<-epic1Running

	// Wait for epic two's real (second) Acquire call to have started.
	<-epic2SecondAcquireStarted

	// The synchronization point the ticket asks for: prove the block is
	// genuine rather than "happened to run second". Because the registry's
	// cap is one and epic one has not released (epic1Proceed is still open),
	// epic two's Acquire() *cannot* have returned yet — not a timing
	// assumption, a mutex/condvar invariant the registry itself guarantees.
	select {
	case <-epic2SecondAcquireDone:
		t.Fatal("epic two's second Acquire() returned before epic one released its permit — cap not enforced")
	default:
	}

	// Let epic one finish its iteration and release the permit; epic two's
	// blocked Acquire() can now return.
	close(epic1Proceed)
	<-epic2SecondAcquireDone

	wg.Wait()

	if err1 != nil {
		t.Errorf("epic-one Run() error = %v, want nil", err1)
	}
	if err2 != nil {
		t.Errorf("epic-two Run() error = %v, want nil", err2)
	}

	if max := atomic.LoadInt32(&sharedMaxHeld); max > 1 {
		t.Errorf("max observed concurrently-held permits across both epics = %d, want <= 1", max)
	}

	for name, path := range map[string]string{"epic-one": ticket1, "epic-two": ticket2} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", path, err)
		}
		if !strings.Contains(string(raw), "status: done") {
			t.Errorf("%s ticket 01 not marked done:\n%s", name, raw)
		}
	}
}
