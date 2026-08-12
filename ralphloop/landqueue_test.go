package ralphloop

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
)

// TestRun_QueuedBehindLandingBuild_ReleasesActiveSlotForNextTicket is the
// land-queue's signature behavior: a build that finished (ahead > 0) must
// hand off to the land-queue worker and free its active slot immediately,
// rather than holding it for the duration of the land (which can take up to
// 30 minutes on a conflict). With MaxParallel: 1, ticket 02 can only start
// building while ticket 01's land is still blocked in CherryPickRange if
// ticket 01's active slot was released at hand-off, not at land completion.
func TestRun_QueuedBehindLandingBuild_ReleasesActiveSlotForNextTicket(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
	})
	d, _, _ := fakeDeps()

	landingStarted := make(chan struct{})
	releaseLanding := make(chan struct{})
	var once sync.Once
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		once.Do(func() { close(landingStarted) })
		<-releaseLanding
		return nil
	}

	ticket02Started := make(chan struct{})
	origTabCreate := d.TabCreate
	d.TabCreate = func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error) {
		if opts.Label == "epic-iter-02" {
			var once2 sync.Once
			once2.Do(func() { close(ticket02Started) })
		}
		return origTabCreate(opts)
	}

	done := make(chan error, 1)
	go func() {
		done <- Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo", MaxParallel: 1}, d, noopEventSink{})
	}()

	select {
	case <-landingStarted:
	case err := <-done:
		t.Fatalf("Run() returned (err=%v) before ticket 01's land ever started", err)
	case <-time.After(10 * time.Second):
		t.Fatal("ticket 01's land never started")
	}

	select {
	case <-ticket02Started:
		// Ticket 02's build started while ticket 01's land was still in
		// flight — proof the build's active slot was released at hand-off
		// rather than held through landing.
	case <-time.After(2 * time.Second):
		t.Fatal("ticket 02 never started building while ticket 01's land was still in flight — active slot held through landing")
	}

	close(releaseLanding)
	if err := <-done; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

// TestRun_TwoLands_NeverRunConcurrently asserts the land-queue worker's
// single-goroutine-ness is what now serializes cherry-pick landings onto the
// feature branch, replacing featureMu: two independent tickets both
// finishing their builds around the same time must never have their
// CherryPickRange calls overlap.
func TestRun_TwoLands_NeverRunConcurrently(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
	})
	d, _, _ := fakeDeps()

	var inFlight atomic.Int32
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		if inFlight.Add(1) > 1 {
			t.Errorf("CherryPickRange called concurrently — land-queue worker must serialize landings")
		}
		time.Sleep(20 * time.Millisecond)
		inFlight.Add(-1)
		return nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

// TestRun_ExitsOnlyAfterLastTicketsLandCompletes covers the new landing
// counter: once the epic's only ticket finishes building, active drops to 0
// while its land is still queued/in-flight. Before the landing counter, that
// state (active == 0, nothing else runnable) looked identical to "done" or
// "deadlocked" — Run must instead keep waiting for the land to actually
// finish before exiting.
func TestRun_ExitsOnlyAfterLastTicketsLandCompletes(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()

	releaseLanding := make(chan struct{})
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		<-releaseLanding
		return nil
	}

	done := make(chan error, 1)
	go func() {
		done <- Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})
	}()

	select {
	case err := <-done:
		t.Fatalf("Run() returned early (err=%v) before the only ticket's land finished — active==0 with a land still in flight must not look like exit-ready or deadlocked", err)
	case <-time.After(300 * time.Millisecond):
	}

	close(releaseLanding)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() never returned after the land finished")
	}
}
