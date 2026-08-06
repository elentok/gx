package tickets

import "testing"

func TestEpic_RenderedStatus_BaseStates(t *testing.T) {
	cases := []struct {
		status string
		want   RenderedStatus
	}{
		{"", StatusOpen},
		{"open", StatusOpen},
		{"ready-for-agent", StatusOpen},
		{"ready-for-human", StatusOpen},
		{"claimed", StatusClaimed},
		{"needs-info", StatusNeedsInfo},
		{"needs-attention", StatusNeedsAttention},
		{"needs-triage", StatusOpen},
		{"done", StatusDone},
		{"resolved", StatusDone},
		{"wontfix", StatusDone},
		{"closed", StatusDone},
		{"superseded", StatusSuperseded},
		{"CLAIMED", StatusClaimed},
		{"bogus-value", StatusError},
	}

	for _, c := range cases {
		epic := Epic{Tickets: []Ticket{{Number: 1, Status: c.status}}}
		got := epic.RenderedStatus(epic.Tickets[0])
		if got != c.want {
			t.Errorf("RenderedStatus(Status: %q) = %v, want %v", c.status, got, c.want)
		}
	}
}

func TestEpic_RenderedStatus_ReadErrIsError(t *testing.T) {
	epic := Epic{Tickets: []Ticket{{Number: 1, Status: "open", ReadErr: "permission denied"}}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusError {
		t.Errorf("RenderedStatus(ReadErr set) = %v, want StatusError", got)
	}
}

func TestEpic_RenderedStatus_BlockedOverlaysOpenAndClaimed(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Status: "", BlockedBy: []string{"2"}},
		{Number: 2, Status: "open"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusBlocked {
		t.Errorf("RenderedStatus = %v, want StatusBlocked", got)
	}

	claimedEpic := Epic{Tickets: []Ticket{
		{Number: 1, Status: "claimed", BlockedBy: []string{"2"}},
		{Number: 2, Status: "open"},
	}}
	got = claimedEpic.RenderedStatus(claimedEpic.Tickets[0])
	if got != StatusBlocked {
		t.Errorf("RenderedStatus (claimed base) = %v, want StatusBlocked", got)
	}
}

func TestEpic_RenderedStatus_ResolvedBlockerDropsOverlay(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Status: "open", BlockedBy: []string{"2"}},
		{Number: 2, Status: "done"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusOpen {
		t.Errorf("RenderedStatus = %v, want StatusOpen once blocker is done", got)
	}
}

func TestEpic_RenderedStatus_NeedsInfoNotOverlaidByBlocked(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Status: "needs-info", BlockedBy: []string{"2"}},
		{Number: 2, Status: "open"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusNeedsInfo {
		t.Errorf("RenderedStatus = %v, want StatusNeedsInfo (blocked overlay only applies to open/claimed)", got)
	}
}

func TestEpic_RenderedStatus_NeedsAttentionNotOverlaidByBlocked(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Status: "needs-attention", BlockedBy: []string{"2"}},
		{Number: 2, Status: "open"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusNeedsAttention {
		t.Errorf("RenderedStatus = %v, want StatusNeedsAttention", got)
	}
}

func TestEpic_RenderedStatus_DoneIgnoresBlockedBy(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Status: "done", BlockedBy: []string{"2"}},
		{Number: 2, Status: "open"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusDone {
		t.Errorf("RenderedStatus = %v, want StatusDone", got)
	}
}

func TestEpic_UnresolvedBlockers(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"2", "3", "4"}},
		{Number: 2, Status: "done"},
		{Number: 3, Status: "open"},
		// 4 doesn't exist in the epic: treated as still unresolved.
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	want := []string{"3", "4"}
	if len(got) != len(want) {
		t.Fatalf("UnresolvedBlockers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("UnresolvedBlockers = %v, want %v", got, want)
		}
	}
}

func TestEpic_UnresolvedBlockers_NoneWhenAllResolved(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"2"}},
		{Number: 2, Status: "resolved"},
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	if got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil", got)
	}
}

func TestEpic_UnresolvedBlockers_NilWhenNoBlockedBy(t *testing.T) {
	epic := Epic{Tickets: []Ticket{{Number: 1}}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	if got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil", got)
	}
}

// TestEpic_UnresolvedBlockers_SelfExcludedFromOwnFamily covers a follow-up
// ticket (06b) that merely shares its prerequisite's leading number (06),
// rather than being one of 06's lettered split replacements. Its own
// "Blocked by: 06" must resolve once 06 itself is done, without also
// requiring 06b — the very ticket being checked, necessarily still open for
// the duration of this check — to finish first.
func TestEpic_UnresolvedBlockers_SelfExcludedFromOwnFamily(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 6, Identifier: "06", Status: "done"},
		{Number: 6, Identifier: "06b", BlockedBy: []string{"06"}, Status: "open"},
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[1])
	if got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil once 06 is done, regardless of 06b's own status", got)
	}
}

// TestEpic_UnresolvedBlockers_LetteredSplitRequiresAllSiblingsDone covers a
// mid-flight split: "Blocked by:" text only ever carries a bare number
// (parseBlockedBy strips letter suffixes), so "Blocked by: 03" can't name a
// specific lettered sibling — it means the whole family sharing Number 3.
// The superseded original (03) is closed as done immediately at split time,
// well before its replacements (03a, 03b) land, so a blocker on "3" must
// stay unresolved until every ticket sharing that Number is done, not just
// the first one.
func TestEpic_UnresolvedBlockers_LetteredSplitRequiresAllSiblingsDone(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"3"}},
		{Number: 3, Identifier: "03", Status: "done"}, // superseded original, closed at split time
		{Number: 3, Identifier: "03a", Status: "done"},
		{Number: 3, Identifier: "03b", Status: "open"}, // still in flight
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	if len(got) != 1 || got[0] != "3" {
		t.Fatalf("UnresolvedBlockers = %v, want [3] while 03b is still open", got)
	}

	epic.Tickets[3].Status = "done"
	got = epic.UnresolvedBlockers(epic.Tickets[0])
	if got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil once every ticket sharing Number 3 is done", got)
	}
}

// TestEpic_UnresolvedBlockers_LetteredTokenNamesOneSibling covers the
// opposite of the bare-number case above: "Blocked by: 03a" names one
// specific split sibling, so it resolves as soon as that ticket alone is
// done — it must not require its still-open siblings (03b) or the
// superseded original (03) to finish too, unlike a bare "Blocked by: 3".
func TestEpic_UnresolvedBlockers_LetteredTokenNamesOneSibling(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"3a"}}, // no zero-padding, unlike Identifier "03a" below
		{Number: 3, Identifier: "03", Status: "superseded"},
		{Number: 3, Identifier: "03a", Status: "done"},
		{Number: 3, Identifier: "03b", Status: "open"}, // still in flight, doesn't matter
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	if got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil — 03a alone is done", got)
	}
}

// TestEpic_UnresolvedBlockers_SplitSiblingsDontBlockEachOther covers 05's
// real-world split: both 05b and 05c inherit "Blocked by: 05" from the
// split, and share Number 5 with each other. Without excluding split
// siblings from the family count, each would need the other done too,
// deadlocking them against each other despite 05 itself being done.
func TestEpic_UnresolvedBlockers_SplitSiblingsDontBlockEachOther(t *testing.T) {
	original := "05"
	epic := Epic{Tickets: []Ticket{
		{Number: 5, Identifier: "05", Status: "done", Split: []string{"05b", "05c"}},
		{Number: 5, Identifier: "05b", BlockedBy: []string{"05"}, Parent: &original, Status: "ready-for-agent"},
		{Number: 5, Identifier: "05c", BlockedBy: []string{"05"}, Parent: &original, Status: "ready-for-agent"},
	}}
	if got := epic.UnresolvedBlockers(epic.Tickets[1]); got != nil {
		t.Errorf("UnresolvedBlockers(05b) = %v, want nil since 05 is done", got)
	}
	if got := epic.UnresolvedBlockers(epic.Tickets[2]); got != nil {
		t.Errorf("UnresolvedBlockers(05c) = %v, want nil since 05 is done", got)
	}
}

func TestEpic_BlockingTickets_ResolvesTokensToTicketsForModal(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", BlockedBy: []string{"2", "3"}},
		{Number: 2, Identifier: "02", Title: "Second ticket", Status: "open"},
		{Number: 3, Identifier: "03", Title: "Third ticket", Status: "claimed"},
	}}
	got := epic.BlockingTickets(epic.Tickets[0])
	if len(got) != 2 || got[0].Identifier != "02" || got[1].Identifier != "03" {
		t.Fatalf("BlockingTickets = %+v, want [02 Second ticket, 03 Third ticket]", got)
	}
}

func TestEpic_BlockingTickets_NilWhenNothingUnresolved(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"2"}},
		{Number: 2, Status: "done"},
	}}
	if got := epic.BlockingTickets(epic.Tickets[0]); got != nil {
		t.Errorf("BlockingTickets = %v, want nil", got)
	}
}

// TestEpic_BlockingTickets_BareNumberResolvesEveryNotYetDoneSibling covers a
// mid-flight split blocker (see UnresolvedBlockers' bare-number family
// semantics): "Blocked by: 3" should surface every one of 3's not-yet-done
// siblings, not just the superseded original, so confirming the modal adds
// them all to the checked set.
func TestEpic_BlockingTickets_BareNumberResolvesEveryNotYetDoneSibling(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"3"}},
		{Number: 3, Identifier: "03", Title: "Original", Status: "done"},
		{Number: 3, Identifier: "03a", Title: "Split A", Status: "done"},
		{Number: 3, Identifier: "03b", Title: "Split B", Status: "open"},
	}}
	got := epic.BlockingTickets(epic.Tickets[0])
	if len(got) != 1 || got[0].Identifier != "03b" {
		t.Fatalf("BlockingTickets = %+v, want [03b Split B]", got)
	}
}

func TestEpic_BlockingTickets_LetteredTokenResolvesOnlyThatSibling(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"3a"}},
		{Number: 3, Identifier: "03", Status: "superseded"},
		{Number: 3, Identifier: "03a", Title: "Split A", Status: "open"},
		{Number: 3, Identifier: "03b", Title: "Split B", Status: "open"},
	}}
	got := epic.BlockingTickets(epic.Tickets[0])
	if len(got) != 1 || got[0].Identifier != "03a" {
		t.Fatalf("BlockingTickets = %+v, want [03a Split A]", got)
	}
}
