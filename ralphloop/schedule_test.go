package ralphloop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestFrontier_MixedStatuses(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 3, Status: "draft"},
		{Number: 1, Status: "open"},
		{Number: 2, Status: "claimed"},
		{Number: 4, Status: "done"},
		{Number: 5, Status: "needs-info"},
	}}

	got := Frontier(epic)
	want := []int{1}
	assertNumbers(t, got, want)
}

func TestFrontier_PartiallyBlockedIsExcluded(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, BlockedBy: []string{"2", "3"}},
		{Number: 2, Status: "done"},
		{Number: 3, Status: "open"}, // still open, so 1 stays blocked
	}}

	got := Frontier(epic)
	assertNumbers(t, got, []int{3})
}

func TestFrontier_ClaimedAndDoneFamilyExcluded(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "claimed"},
		{Number: 2, Status: "done"},
		{Number: 3, Status: "resolved"},
		{Number: 4, Status: "wontfix"},
		{Number: 5, Status: "closed"},
		{Number: 6, Status: "done", Commitless: true},
		{Number: 7, Status: "implemented"},
		{Number: 8, Status: "needs-info"},
		{Number: 9, Status: "open"},
	}}

	got := Frontier(epic)
	assertNumbers(t, got, []int{9})
}

func TestFrontier_OrderedByTicketNumber(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 5, Status: "open"},
		{Number: 1, Status: "open"},
		{Number: 3, Status: "open"},
	}}

	got := Frontier(epic)
	assertNumbers(t, got, []int{1, 3, 5})
}

func TestFrontier_BlockerChain(t *testing.T) {
	// 1 <- 2 <- 3: only 1 is unblocked until each link resolves in turn.
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "open"},
		{Number: 2, Status: "open", BlockedBy: []string{"1"}},
		{Number: 3, Status: "open", BlockedBy: []string{"2"}},
	}}
	assertNumbers(t, Frontier(epic), []int{1})

	epic.Tickets[0].Status = "done"
	assertNumbers(t, Frontier(epic), []int{2})

	epic.Tickets[1].Status = "done"
	assertNumbers(t, Frontier(epic), []int{3})
}

// TestFrontier_NoStatusIsNotSchedulable pins the contraction's central
// safety property: status is required, so a ticket with none is an error
// rather than an open ticket an agent can be handed.
func TestFrontier_NoStatusIsNotSchedulable(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{{Number: 1}}}
	assertNumbers(t, Frontier(epic), nil)
}

func TestFrontier_EmptyEpic(t *testing.T) {
	got := Frontier(tickets.Epic{})
	if len(got) != 0 {
		t.Errorf("Frontier(empty epic) = %v, want empty", got)
	}
}

// TestFrontier_AgainstFixtureEpicDirectory exercises Frontier end-to-end
// through tickets.Load against real files on disk, not just in-memory Epic
// literals, per the ticket's "fully unit-testable against fixture ticket
// directories" requirement.
func TestFrontier_AgainstFixtureEpicDirectory(t *testing.T) {
	scratchDir := t.TempDir()
	issuesDir := filepath.Join(scratchDir, "my-epic", "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	files := map[string]string{
		"01-first.md":       "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
		"02-second.md":      "---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# Second\n",
		"03-third.md":       "---\nid: \"03\"\nstatus: open\ntype: task\n---\n# Third\n",
		"04-fourth.md":      "---\nid: \"04\"\nstatus: claimed\ntype: task\n---\n# Fourth\n",
		"05-fifth.md":       "---\nid: \"05\"\nstatus: done\ntype: task\n---\n# Fifth\n",
		"06-blocked-off.md": "---\nid: \"06\"\nstatus: open\ntype: task\nblocked_by: [\"04\"]\n---\n# Blocked off\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(issuesDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}
	if len(epics) != 1 {
		t.Fatalf("tickets.Load returned %d epics, want 1", len(epics))
	}

	got := Frontier(epics[0])
	// 02 stays blocked (its blocker 01 is open, not done); 03 is open with no
	// blockers; 04/claimed, 05/done, and 06 (blocked on the claimed 04) are
	// all excluded.
	assertNumbers(t, got, []int{1, 3})
}

func TestFrontier_DraftIsExcluded(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "draft"},
		{Number: 2, Status: "open"},
	}}

	assertNumbers(t, Frontier(epic), []int{2})
}

func TestFrontier_TicketBlockedOnDraftStaysBlocked(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "draft"},
		{Number: 2, Status: "open", BlockedBy: []string{"1"}},
	}}

	if got := Frontier(epic); len(got) != 0 {
		t.Fatalf("Frontier() = %v, want empty: a draft blocker is not done", numbers(got))
	}
}

func assertNumbers(t *testing.T, got []tickets.Ticket, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d tickets %v, want %d %v", len(got), numbers(got), len(want), want)
	}
	for i, n := range want {
		if got[i].Number != n {
			t.Fatalf("got %v, want %v", numbers(got), want)
		}
	}
}

func numbers(ts []tickets.Ticket) []int {
	ns := make([]int, len(ts))
	for i, t := range ts {
		ns[i] = t.Number
	}
	return ns
}
