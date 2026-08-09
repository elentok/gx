package ralphloop

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// clearOnPark returns a Deps.Sleep replacement for a parked run: the first
// call rewrites clearPath's status to newStatus, so the park's next pass sees
// a ticket a "human" cleared while it was waiting. Without it a parked run
// polls forever, which is the point of the feature and the hazard of testing
// it — every park test needs some scripted clearing hand.
func clearOnPark(t *testing.T, clearPath, newStatus string) (func(time.Duration), *int) {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	return func(dur time.Duration) {
		// Deps.Sleep backs several poll loops; parkPollInterval is what tells
		// a park tick apart from an iteration's own waiting.
		if dur != parkPollInterval {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls != 1 {
			return
		}
		raw, err := os.ReadFile(clearPath)
		if err != nil {
			t.Errorf("ReadFile %s: %v", clearPath, err)
			return
		}
		if err := SetStatus(clearPath, newStatus); err != nil {
			t.Errorf("SetStatus %s: %v (was %q)", clearPath, err, raw)
		}
	}, &calls
}

func ticketPath(scratchDir, epicName, file string) string {
	return filepath.Join(scratchDir, epicName, "issues", file)
}

// TestRun_StalledTicket_ParksInsteadOfExiting covers ticket 08's core AC: a
// run whose only remaining ticket is needs-info neither exits nor errors — it
// parks, notifies, and carries on once the status clears.
func TestRun_StalledTicket_ParksInsteadOfExiting(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-stuck.md": "---\nid: \"01\"\nstatus: needs-info\ntype: task\n---\n# Stuck\n",
	})
	d, prompts, _ := fakeDeps()
	sleep, sleeps := clearOnPark(t, ticketPath(scratchDir, "my-epic", "01-stuck.md"), "open")
	d.Sleep = sleep
	// The park has no timeout: only the scripted clearing hand above ends it.
	d.maxParkPolls = 0
	sink := &recordingSink{}

	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (a stalled run parks, it doesn't error)", err)
	}

	if *sleeps == 0 {
		t.Errorf("run never parked (no park poll), want it to park on the needs-info ticket")
	}
	if len(sink.parkedStalled) != 1 || len(sink.parkedStalled[0]) != 1 || sink.parkedStalled[0][0] != "01" {
		t.Errorf("EpicParked calls = %v, want one naming ticket 01", sink.parkedStalled)
	}
	if len(*prompts) != 1 {
		t.Errorf("prompts = %v, want the cleared ticket to be claimed and run once", *prompts)
	}
}

// TestRun_NothingRunnableAndNothingClearable_Deadlocks keeps the genuine
// corruption signal: a blocker naming a ticket that will never resolve has no
// human-clearable ticket to park on, so it must still error.
func TestRun_NothingRunnableAndNothingClearable_Deadlocks(t *testing.T) {
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
