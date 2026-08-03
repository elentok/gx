package ralphloop

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// classifyDoneTicketFixture is the shared setup for the four
// classifyDoneTicket outcomes: one done ticket, plus a Deps whose
// RevParse/IsAncestor/WorktreeExists are overridden per-case below.
func classifyDoneTicketFixture() (Deps, tickets.Ticket) {
	d, _, _ := fakeDeps()
	return d, tickets.Ticket{Number: 3, Identifier: "03", Status: "done"}
}

func TestClassifyDoneTicket_CommitLandedNoLeftover_OK(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return true, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}}

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, events, map[string]bool{}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneOK {
		t.Errorf("class = %v, want doneOK", class)
	}
}

func TestClassifyDoneTicket_CommitLandedButBranchLeftover_StaleCleanup(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return true, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "deadbeef", nil } // iteration branch still exists
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}}

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, events, map[string]bool{}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneStaleCleanup {
		t.Errorf("class = %v, want doneStaleCleanup", class)
	}
}

func TestClassifyDoneTicket_CommitMissingBranchStillHasIt_Recoverable(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "deadbeef", nil }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}}

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, events, map[string]bool{}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneRecoverable {
		t.Errorf("class = %v, want doneRecoverable", class)
	}
}

func TestClassifyDoneTicket_CommitMissingNoBranch_Unrecoverable(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}}

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, events, map[string]bool{}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneUnrecoverable {
		t.Errorf("class = %v, want doneUnrecoverable", class)
	}
}

func TestClassifyDoneTicket_NoRecordedEvent_TreatedAsMissing(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) {
		t.Fatal("IsAncestor should not be called with no recorded SHA to check")
		return false, nil
	}
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, nil, map[string]bool{}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneUnrecoverable {
		t.Errorf("class = %v, want doneUnrecoverable when no event was ever logged", class)
	}
}

// TestIterLabelIterBranch_DistinctForLetteredSiblingsSharingNumber covers
// ticket 08: a mid-flight split keeps the same Number across every letter
// (e.g. 04 -> 04a/04b), so iterLabel/iterBranch must key off Identifier, not
// Number, or lettered siblings would collide on the same worktree/branch.
func TestIterLabelIterBranch_DistinctForLetteredSiblingsSharingNumber(t *testing.T) {
	a, b := tickets.Ticket{Number: 4, Identifier: "04a"}, tickets.Ticket{Number: 4, Identifier: "04b"}
	if iterLabel(a.Identifier) == iterLabel(b.Identifier) {
		t.Errorf("iterLabel(%q) == iterLabel(%q) = %q, want distinct labels for siblings sharing Number 4", a.Identifier, b.Identifier, iterLabel(a.Identifier))
	}
	if iterBranch("epic", a.Identifier) == iterBranch("epic", b.Identifier) {
		t.Errorf("iterBranch(%q) == iterBranch(%q) = %q, want distinct branches for siblings sharing Number 4", a.Identifier, b.Identifier, iterBranch("epic", a.Identifier))
	}
}

// TestClassifyDoneTicket_LetteredSiblingsShareNumber_NotCrossAttributed
// covers the other half of ticket 08: classifyDoneTicket/latestCherryPickedSHA
// must attribute a run-log cherry-picked event to the sibling that logged it
// (by Identifier), not whichever sibling happens to share its Number. Two
// done tickets, 04a and 04b, both Number 4: 04a's commit landed on the
// feature branch, 04b's didn't (and its iteration branch is gone). If SHA
// lookup were still keyed by Number, 04b would inherit 04a's landed SHA and
// misreport doneOK instead of doneUnrecoverable.
func TestClassifyDoneTicket_LetteredSiblingsShareNumber_NotCrossAttributed(t *testing.T) {
	d, _, _ := fakeDeps()
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) {
		return ancestor == "sha-04a", nil // only 04a's commit actually landed
	}
	d.RevParse = func(dir, ref string) (string, error) {
		return "", fmt.Errorf("unknown revision") // neither iteration branch survives
	}

	events := []Event{
		{Type: eventCherryPicked, Ticket: "04a", SHA: "sha-04a"},
		{Type: eventCherryPicked, Ticket: "04b", SHA: "sha-04b"},
	}
	paths := reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}

	classA, err := classifyDoneTicket(d, paths, "epic", tickets.Ticket{Number: 4, Identifier: "04a", Status: "done"}, events, map[string]bool{}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket(04a) error = %v", err)
	}
	if classA != doneOK {
		t.Errorf("classA = %v, want doneOK for 04a's own landed commit", classA)
	}

	classB, err := classifyDoneTicket(d, paths, "epic", tickets.Ticket{Number: 4, Identifier: "04b", Status: "done"}, events, map[string]bool{}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket(04b) error = %v", err)
	}
	if classB != doneUnrecoverable {
		t.Errorf("classB = %v, want doneUnrecoverable — 04b must not inherit 04a's landed SHA just because they share Number 4", classB)
	}
}

func TestClassifyDoneTicket_LiveTabCountsAsLeftover(t *testing.T) {
	d, ticket := classifyDoneTicketFixture()
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return true, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }
	d.WorktreeExists = func(path string) (bool, error) { return false, nil }
	events := []Event{{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}}

	class, err := classifyDoneTicket(d, reconcilePaths{FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, "epic", ticket, events, map[string]bool{iterationKey("epic", iterLabel("03")): true}, map[string]bool{})
	if err != nil {
		t.Fatalf("classifyDoneTicket() error = %v", err)
	}
	if class != doneStaleCleanup {
		t.Errorf("class = %v, want doneStaleCleanup when the iteration tab is still live", class)
	}
}

// TestReconcile_DoneTicketMismatch_ReportedNotRepaired exercises the full
// reconcile() entrypoint (not just classifyDoneTicket in isolation): a done
// ticket whose landed commit went missing is reported, but reconcile doesn't
// touch its status or the reattached list — repair is a later ticket's job.
// TestReconcile_DoneTicketUnrecoverable_MarkedNeedsAttention exercises ticket
// 03: a done ticket classified doneUnrecoverable (its landed commit missing
// from the feature branch, no iteration branch left to recover it from) must
// not be silently reverted to open or left marked done — it's flagged
// needs-attention for a human to inspect, with a reason and a logged event.
func TestReconcile_DoneTicketUnrecoverable_MarkedNeedsAttention(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"03-c.md": "---\nid: \"03\"\nstatus: done\ntype: task\n---\n# C\n",
	})
	if err := logEvent(scratchDir, "epic", Event{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}); err != nil {
		t.Fatalf("logEvent: %v", err)
	}
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) { return nil, nil }
	d.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, nil }
	d.RevParse = func(dir, ref string) (string, error) { return "", fmt.Errorf("unknown revision") }

	var out bytes.Buffer
	reattached, err := reconcile(d, testReconcileParams("ws1", reconcilePaths{ScratchDir: scratchDir, FeatureWorktree: "/fake/feature", WorktreeDir: "/fake/worktrees"}, NewTextEventSink(&out)), epics[0])
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Errorf("reattached = %v, want none for a done ticket", reattached)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "03-c.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-attention") {
		t.Errorf("ticket not marked needs-attention:\n%s", raw)
	}

	reports := strings.Split(out.String(), "\n")
	found := false
	for _, r := range reports {
		if strings.Contains(r, "ticket 03") && strings.Contains(r, "no iteration branch left") {
			found = true
		}
	}
	if !found {
		t.Errorf("reports = %v, want an unrecoverable-mismatch report for ticket 03", reports)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("readEvents: %v", err)
	}
	if !ok {
		t.Fatalf("readEvents: run log not found")
	}
	var attentionEvent *Event
	for i := range events {
		if events[i].Type == eventNeedsAttention && events[i].Ticket == "03" {
			attentionEvent = &events[i]
		}
	}
	if attentionEvent == nil {
		t.Fatalf("events = %v, want a needs-attention event for ticket 3", events)
	}
	if attentionEvent.Reason == "" {
		t.Errorf("needs-attention event has no reason")
	}
}
