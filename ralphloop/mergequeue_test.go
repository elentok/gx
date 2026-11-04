package ralphloop

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
)

// TestRun_QueuedBehindMergingBuild_ReleasesActiveSlotForNextTicket is the
// merge-queue's signature behavior: a build that finished (ahead > 0) must
// hand off to the merge-queue worker and free its active slot immediately,
// rather than holding it for the duration of the merge (which can take up to
// 30 minutes on a conflict). With MaxParallel: 1, ticket 02 can only start
// building while ticket 01's merge is still blocked in CherryPickRange if
// ticket 01's active slot was released at hand-off, not at merge completion.
func TestRun_QueuedBehindMergingBuild_ReleasesActiveSlotForNextTicket(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n# B\n",
	})
	d, _, _ := fakeDeps()

	mergingStarted := make(chan struct{})
	releaseMerging := make(chan struct{})
	var once sync.Once
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		once.Do(func() { close(mergingStarted) })
		<-releaseMerging
		return nil
	}

	ticket02Started := make(chan struct{})
	onRunnerStart(d, func(opts agentrunner.StartOptions) error {
		if opts.Label == "epic-iter-02" {
			var once2 sync.Once
			once2.Do(func() { close(ticket02Started) })
		}
		return nil
	})

	done := make(chan error, 1)
	go func() {
		done <- Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo", MaxParallel: 1}, d, noopEventSink{})
	}()

	select {
	case <-mergingStarted:
	case err := <-done:
		t.Fatalf("Run() returned (err=%v) before ticket 01's merge ever started", err)
	case <-time.After(10 * time.Second):
		t.Fatal("ticket 01's merge never started")
	}

	select {
	case <-ticket02Started:
		// Ticket 02's build started while ticket 01's merge was still in
		// flight — proof the build's active slot was released at hand-off
		// rather than held through merging.
	case <-time.After(2 * time.Second):
		t.Fatal("ticket 02 never started building while ticket 01's merge was still in flight — active slot held through merging")
	}

	close(releaseMerging)
	if err := <-done; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

// TestRun_TwoMerges_NeverRunConcurrently asserts the merge-queue worker's
// single-goroutine-ness is what now serializes cherry-pick merges onto the
// feature branch, replacing featureMu: two independent tickets both
// finishing their builds around the same time must never have their
// CherryPickRange calls overlap.
func TestRun_TwoMerges_NeverRunConcurrently(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n# B\n",
	})
	d, _, _ := fakeDeps()

	var inFlight atomic.Int32
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		if inFlight.Add(1) > 1 {
			t.Errorf("CherryPickRange called concurrently — merge-queue worker must serialize merges")
		}
		time.Sleep(20 * time.Millisecond)
		inFlight.Add(-1)
		return nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

// TestRun_ExitsOnlyAfterLastTicketsMergeCompletes covers the new merging
// counter: once the epic's only ticket finishes building, active drops to 0
// while its merge is still queued/in-flight. Before the merging counter, that
// state (active == 0, nothing else runnable) looked identical to "done" or
// "deadlocked" — Run must instead keep waiting for the merge to actually
// finish before exiting.
func TestRun_ExitsOnlyAfterLastTicketsMergeCompletes(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, _ := fakeDeps()

	releaseMerging := make(chan struct{})
	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		<-releaseMerging
		return nil
	}

	done := make(chan error, 1)
	go func() {
		done <- Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})
	}()

	select {
	case err := <-done:
		t.Fatalf("Run() returned early (err=%v) before the only ticket's merge finished — active==0 with a merge still in flight must not look like exit-ready or deadlocked", err)
	case <-time.After(300 * time.Millisecond):
	}

	close(releaseMerging)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() never returned after the merge finished")
	}
}
