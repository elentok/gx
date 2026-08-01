package ralphloop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestFrontier_MixedStatuses(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 3, Status: ""},
		{Number: 1, Status: "open"},
		{Number: 2, Status: "claimed"},
		{Number: 4, Status: "done"},
		{Number: 5, Status: "needs-info"},
	}}

	got := Frontier(epic)
	want := []int{1, 3}
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
		{Number: 6, Status: "superseded"},
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

func TestFrontier_NoStatusLineIsOpenDefault(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{{Number: 1}}}
	assertNumbers(t, Frontier(epic), []int{1})
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
		"01-first.md":       "# First\n\n**Status:** open\n",
		"02-second.md":      "# Second\n\n**Blocked by:** 01\n\n**Status:** open\n",
		"03-third.md":       "# Third\n\nNo status line at all.\n",
		"04-fourth.md":      "# Fourth\n\n**Status:** claimed\n",
		"05-fifth.md":       "# Fifth\n\n**Status:** done\n",
		"06-blocked-off.md": "# Blocked off\n\n**Blocked by:** 04\n\n**Status:** open\n",
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
	// 02 stays blocked (its blocker 01 is open, not done); 03 has no Status:
	// line (valid open default); 04/claimed, 05/done, and 06 (blocked on the
	// claimed 04) are all excluded.
	assertNumbers(t, got, []int{1, 3})
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
