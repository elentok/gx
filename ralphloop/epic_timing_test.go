package ralphloop

import (
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
)

// loadEpicByName loads scratchDir and returns the named epic, failing the
// test if it isn't found.
func loadEpicByName(t *testing.T, scratchDir, name string) tickets.Epic {
	t.Helper()
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}
	for _, e := range epics {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("epic %q not found in %q", name, scratchDir)
	return tickets.Epic{}
}

// TestRun_StampsEpicStartedAndCompletedAt covers ticket 06's core
// requirement: a fresh run of a whole epic stamps epic.yaml's started_at the
// moment the first ticket is claimed, and completed_at once the last ticket
// (and thus the whole epic, not just this run's scope) finishes.
func TestRun_StampsEpicStartedAndCompletedAt(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# Second\n",
	})
	d, _, _ := fakeDeps()
	fixedNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	d.Now = func() time.Time { return fixedNow }

	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	epic := loadEpicByName(t, scratchDir, "my-epic")
	if !epic.StartedAt.Equal(fixedNow) {
		t.Errorf("StartedAt = %v, want %v", epic.StartedAt, fixedNow)
	}
	if !epic.CompletedAt.Equal(fixedNow) {
		t.Errorf("CompletedAt = %v, want %v", epic.CompletedAt, fixedNow)
	}
}

// TestRun_ReRunOnAlreadyCompleteEpic_DoesNotOverwriteTimestamps covers the
// idempotency half of ticket 06's ACs: re-running (or reattaching to) an
// epic that already finished must leave its already-stamped started_at and
// completed_at untouched, even though the second run's clock has moved on.
func TestRun_ReRunOnAlreadyCompleteEpic_DoesNotOverwriteTimestamps(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	d, _, _ := fakeDeps()
	firstRun := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	d.Now = func() time.Time { return firstRun }

	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() (first) error = %v", err)
	}

	secondRun := firstRun.Add(24 * time.Hour)
	d.Now = func() time.Time { return secondRun }
	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() (second) error = %v", err)
	}

	epic := loadEpicByName(t, scratchDir, "my-epic")
	if !epic.StartedAt.Equal(firstRun) {
		t.Errorf("StartedAt = %v, want unchanged %v", epic.StartedAt, firstRun)
	}
	if !epic.CompletedAt.Equal(firstRun) {
		t.Errorf("CompletedAt = %v, want unchanged %v", epic.CompletedAt, firstRun)
	}
}

// TestRun_TicketSubset_LeavesCompletedAtUnset covers the "epic's last
// ticket" half of the ticket's completed_at rule: a scoped run
// (RunOptions.TicketIDs) that finishes its subset while other epic tickets
// remain open must not stamp completed_at — the epic itself isn't done.
func TestRun_TicketSubset_LeavesCompletedAtUnset(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Second\n",
	})
	d, _, _ := fakeDeps()

	if err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  []string{"01"},
	}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	epic := loadEpicByName(t, scratchDir, "my-epic")
	if epic.StartedAt.IsZero() {
		t.Errorf("StartedAt = zero, want it stamped once ticket 01 was claimed")
	}
	if !epic.CompletedAt.IsZero() {
		t.Errorf("CompletedAt = %v, want zero (ticket 02 is still open outside the subset)", epic.CompletedAt)
	}
}
