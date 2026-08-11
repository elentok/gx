package tickets

import "testing"

func TestEpic_RenderedStatus_BaseStates(t *testing.T) {
	cases := []struct {
		status string
		want   RenderedStatus
	}{
		{"", StatusError},
		{"open", StatusOpen},
		{"ready-for-agent", StatusError},
		{"ready-for-human", StatusError},
		{"claimed", StatusClaimed},
		{"needs-answer", StatusNeedsAnswer},
		{"needs-repair", StatusNeedsRepair},
		{"needs-triage", StatusError},
		{"done", StatusDone},
		{"draft", StatusDraft},
		{"resolved", StatusError},
		{"wontfix", StatusError},
		{"closed", StatusError},
		{"implemented", StatusError},
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
		{Number: 1, Status: "open", BlockedBy: []string{"2"}},
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

func TestEpic_RenderedStatus_NeedsAnswerNotOverlaidByBlocked(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Status: "needs-answer", BlockedBy: []string{"2"}},
		{Number: 2, Status: "open"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusNeedsAnswer {
		t.Errorf("RenderedStatus = %v, want StatusNeedsAnswer (blocked overlay only applies to open/claimed)", got)
	}
}

func TestEpic_RenderedStatus_NeedsRepairNotOverlaidByBlocked(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Status: "needs-repair", BlockedBy: []string{"2"}},
		{Number: 2, Status: "open"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusNeedsRepair {
		t.Errorf("RenderedStatus = %v, want StatusNeedsRepair", got)
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
		{Number: 2, Status: "done"},
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

// TestEpic_UnresolvedBlockers_SameNumberFamilyIsNotForkSubtree covers a
// follow-up ticket (06b) that merely shares its prerequisite's leading
// number (06) without being one of 06's Parent-linked fork children. Its own
// "Blocked by: 06" resolves once 06 itself is done — sharing a number alone
// puts nothing in 06's fork subtree, so 06b's own open status can't leak
// into resolving its own token.
func TestEpic_UnresolvedBlockers_SameNumberFamilyIsNotForkSubtree(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 6, Identifier: "06", Status: "done"},
		{Number: 6, Identifier: "06b", BlockedBy: []string{"06"}, Status: "open"},
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[1])
	if got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil once 06 is done, regardless of 06b's own status", got)
	}
}

// TestEpic_UnresolvedBlockers_LinearForkChainWaitsForWholeChain covers a
// dependent blocked on the root of a linear fork chain (X -> Xa -> Xa1):
// Blocking recurses down Parent reverse-edges at any depth, so a bare-number
// token on the root stays unresolved until every descendant, not just the
// immediate child, is done.
func TestEpic_UnresolvedBlockers_LinearForkChainWaitsForWholeChain(t *testing.T) {
	root := "03"
	mid := "03a"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"3"}},
		{Number: 3, Identifier: "03", Status: "done"},
		{Number: 3, Identifier: "03a", Status: "done", Parent: &root},
		{Number: 3, Identifier: "03a1", Status: "open", Parent: &mid},
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	if len(got) != 1 || got[0] != "3" {
		t.Fatalf("UnresolvedBlockers = %v, want [3] while 03a1 is still open", got)
	}

	epic.Tickets[3].Status = "done"
	got = epic.UnresolvedBlockers(epic.Tickets[0])
	if got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil once every ticket in the chain is done", got)
	}
}

// TestEpic_UnresolvedBlockers_ForkWithParallelChildrenRequiresAllDone covers
// a fork with parallel children (03 -> 03a, 03b): a bare-number token on the
// root stays unresolved until every parallel child is done, not just one.
func TestEpic_UnresolvedBlockers_ForkWithParallelChildrenRequiresAllDone(t *testing.T) {
	original := "03"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"3"}},
		{Number: 3, Identifier: "03", Status: "done"},
		{Number: 3, Identifier: "03a", Status: "done", Parent: &original},
		{Number: 3, Identifier: "03b", Status: "open", Parent: &original},
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	if len(got) != 1 || got[0] != "3" {
		t.Fatalf("UnresolvedBlockers = %v, want [3] while 03b is still open", got)
	}

	epic.Tickets[3].Status = "done"
	got = epic.UnresolvedBlockers(epic.Tickets[0])
	if got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil once every child is done", got)
	}
}

// TestEpic_UnresolvedBlockers_LetteredTokenNamesOneSibling covers a token
// naming one specific fork child directly: it resolves as soon as that
// ticket's own subtree is done, without waiting on its still-open siblings.
func TestEpic_UnresolvedBlockers_LetteredTokenNamesOneSibling(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"3a"}}, // no zero-padding, unlike Identifier "03a" below
		{Number: 3, Identifier: "03", Status: "done"},
		{Number: 3, Identifier: "03a", Status: "done"},
		{Number: 3, Identifier: "03b", Status: "open"}, // still in flight, doesn't matter
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	if got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil — 03a alone is done", got)
	}
}

// TestEpic_UnresolvedBlockers_DirectSiblingTokenIsEnforced covers a fork
// child blocked directly on its own sibling (not their shared parent): 02b
// and 02c share Parent 02, so neither is in the other's fork subtree, but
// 02c's own "Blocked by: 02b" still names 02b directly and must be enforced
// against 02b's real status.
func TestEpic_UnresolvedBlockers_DirectSiblingTokenIsEnforced(t *testing.T) {
	original := "02"
	epic := Epic{Tickets: []Ticket{
		{Number: 2, Identifier: "02", Status: "done"},
		{Number: 2, Identifier: "02b", Parent: &original, Status: "open"},
		{Number: 2, Identifier: "02c", Parent: &original, BlockedBy: []string{"02b"}, Status: "open"},
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[2])
	if len(got) != 1 || got[0] != "02b" {
		t.Fatalf("UnresolvedBlockers(02c) = %v, want [02b] while 02b is still open", got)
	}
	if status := epic.RenderedStatus(epic.Tickets[2]); status != StatusBlocked {
		t.Errorf("RenderedStatus(02c) = %v, want StatusBlocked", status)
	}

	epic.Tickets[1].Status = "done"
	got = epic.UnresolvedBlockers(epic.Tickets[2])
	if got != nil {
		t.Errorf("UnresolvedBlockers(02c) = %v, want nil once 02b is done", got)
	}
	if status := epic.RenderedStatus(epic.Tickets[2]); status != StatusOpen {
		t.Errorf("RenderedStatus(02c) = %v, want StatusOpen once 02b is done", status)
	}
}

// TestEpic_UnresolvedBlockers_NestedForkDepthDoesntMatter covers a fork
// within a fork (01 -> 01a -> 01a1, 01a2): an external dependent blocked on
// the root ("1") waits for the whole nested subtree regardless of how deep
// it goes, and a dependent blocked on the inner fork's root ("01a") waits
// for just that subtree.
func TestEpic_UnresolvedBlockers_NestedForkDepthDoesntMatter(t *testing.T) {
	root := "01"
	mid := "01a"
	epic := Epic{Tickets: []Ticket{
		{Number: 9, BlockedBy: []string{"1"}},
		{Number: 8, Identifier: "08", BlockedBy: []string{"01a"}},
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "done", Parent: &root},
		{Number: 1, Identifier: "01a1", Status: "done", Parent: &mid},
		{Number: 1, Identifier: "01a2", Status: "open", Parent: &mid},
	}}
	if got := epic.UnresolvedBlockers(epic.Tickets[0]); len(got) != 1 || got[0] != "1" {
		t.Fatalf("UnresolvedBlockers(9) = %v, want [1] while 01a2 is still open", got)
	}
	if got := epic.UnresolvedBlockers(epic.Tickets[1]); len(got) != 1 || got[0] != "01a" {
		t.Fatalf("UnresolvedBlockers(08) = %v, want [01a] while 01a2 is still open", got)
	}

	epic.Tickets[5].Status = "done"
	if got := epic.UnresolvedBlockers(epic.Tickets[0]); got != nil {
		t.Errorf("UnresolvedBlockers(9) = %v, want nil once the whole nested subtree is done", got)
	}
	if got := epic.UnresolvedBlockers(epic.Tickets[1]); got != nil {
		t.Errorf("UnresolvedBlockers(08) = %v, want nil once 01a's subtree is done", got)
	}
}

// TestEpic_UnresolvedBlockers_AbsentTicketStaysBlocking covers a Blocked by:
// token naming no ticket in the epic: it can't be verified done, so it
// counts as unresolved forever rather than being silently ignored.
func TestEpic_UnresolvedBlockers_AbsentTicketStaysBlocking(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"99"}},
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	if len(got) != 1 || got[0] != "99" {
		t.Fatalf("UnresolvedBlockers = %v, want [99]: no ticket 99 in this epic", got)
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
// mid-flight fork blocker: "Blocked by: 3" should surface every one of 03's
// not-yet-done fork children, not just the original, so confirming the modal
// adds them all to the checked set.
func TestEpic_BlockingTickets_BareNumberResolvesEveryNotYetDoneSibling(t *testing.T) {
	original := "03"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"3"}},
		{Number: 3, Identifier: "03", Title: "Original", Status: "done"},
		{Number: 3, Identifier: "03a", Title: "Split A", Status: "done", Parent: &original},
		{Number: 3, Identifier: "03b", Title: "Split B", Status: "open", Parent: &original},
	}}
	got := epic.BlockingTickets(epic.Tickets[0])
	if len(got) != 1 || got[0].Identifier != "03b" {
		t.Fatalf("BlockingTickets = %+v, want [03b Split B]", got)
	}
}

func TestEpic_BlockingTickets_LetteredTokenResolvesOnlyThatSibling(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"3a"}},
		{Number: 3, Identifier: "03", Status: "done"},
		{Number: 3, Identifier: "03a", Title: "Split A", Status: "open"},
		{Number: 3, Identifier: "03b", Title: "Split B", Status: "open"},
	}}
	got := epic.BlockingTickets(epic.Tickets[0])
	if len(got) != 1 || got[0].Identifier != "03a" {
		t.Fatalf("BlockingTickets = %+v, want [03a Split A]", got)
	}
}

func TestEpic_Blocking_ChildlessDoneTicketIsNotBlocking(t *testing.T) {
	epic := Epic{Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "done"}}}
	if epic.Blocking(epic.Tickets[0]) {
		t.Errorf("Blocking = true, want false for a childless done ticket")
	}
}

func TestEpic_Blocking_NotDoneItselfIsBlocking(t *testing.T) {
	epic := Epic{Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open"}}}
	if !epic.Blocking(epic.Tickets[0]) {
		t.Errorf("Blocking = false, want true for a ticket whose own status isn't done")
	}
}

func TestEpic_Blocking_DoneWithOpenForkChildIsBlocking(t *testing.T) {
	parent := "01"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "open", Parent: &parent},
	}}
	if !epic.Blocking(epic.Tickets[0]) {
		t.Errorf("Blocking = false, want true: fork child 01a isn't done")
	}
}

func TestEpic_Blocking_DoneWithOpenForkGrandchildIsBlocking(t *testing.T) {
	parent := "01"
	child := "01a"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "done", Parent: &parent},
		{Number: 1, Identifier: "01a-01", Status: "open", Parent: &child},
	}}
	if !epic.Blocking(epic.Tickets[0]) {
		t.Errorf("Blocking = false, want true: grandchild 01a-01 isn't done despite 01 and 01a both being done")
	}
}

func TestEpic_Blocking_DoneWithFullyDoneForkSubtreeIsNotBlocking(t *testing.T) {
	parent := "01"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "done", Parent: &parent},
	}}
	if epic.Blocking(epic.Tickets[0]) {
		t.Errorf("Blocking = true, want false: 01 and its only fork child 01a are both done")
	}
}

func TestEpic_RenderedStatus_DoneWithOpenForkChildIsWaitingForChildren(t *testing.T) {
	parent := "01"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "open", Parent: &parent},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusWaitingForChildren {
		t.Errorf("RenderedStatus = %v, want StatusWaitingForChildren while fork child 01a is still open", got)
	}
}

func TestEpic_RenderedStatus_DoneWithFinishedForkSubtreeIsDone(t *testing.T) {
	parent := "01"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "done", Parent: &parent},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusDone {
		t.Errorf("RenderedStatus = %v, want StatusDone once fork child 01a is done too", got)
	}
}

func TestEpic_RenderedStatus_WaitingForChildrenNotSettled(t *testing.T) {
	parent := "01"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "open", Parent: &parent},
	}}
	if epic.AllDone() {
		t.Errorf("AllDone = true, want false: 01 is waiting-for-children, not settled")
	}
}

func TestEpic_RenderedStatus_CodeReviewBlockedWhileSiblingOpen(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 9, Identifier: "09", Type: typeCodeReview, Status: "open"},
		{Number: 1, Identifier: "01", Status: "open"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusBlocked {
		t.Errorf("RenderedStatus(code-review) = %v, want StatusBlocked while 01 is still open", got)
	}
}

func TestEpic_RenderedStatus_CodeReviewBlockedWhileSiblingClaimed(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 9, Identifier: "09", Type: typeCodeReview, Status: "open"},
		{Number: 1, Identifier: "01", Status: "claimed"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusBlocked {
		t.Errorf("RenderedStatus(code-review) = %v, want StatusBlocked while 01 is still claimed", got)
	}
}

func TestEpic_RenderedStatus_CodeReviewOpenOnceEverySiblingDone(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 9, Identifier: "09", Type: typeCodeReview, Status: "open"},
		{Number: 1, Identifier: "01", Status: "done"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusOpen {
		t.Errorf("RenderedStatus(code-review) = %v, want StatusOpen once every other ticket is done", got)
	}
}

func TestEpic_RenderedStatus_CodeReviewIgnoresOwnBlockedBy(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 9, Identifier: "09", Type: typeCodeReview, Status: "open", BlockedBy: []string{"1"}},
		{Number: 1, Identifier: "01", Status: "done"},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusOpen {
		t.Errorf("RenderedStatus(code-review) = %v, want StatusOpen: its own Blocked by: is irrelevant", got)
	}
}

func TestEpic_RenderedStatus_CodeReviewBlockedByUnfinishedForkSubtree(t *testing.T) {
	parent := "01"
	epic := Epic{Tickets: []Ticket{
		{Number: 9, Identifier: "09", Type: typeCodeReview, Status: "open"},
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "open", Parent: &parent},
	}}
	got := epic.RenderedStatus(epic.Tickets[0])
	if got != StatusBlocked {
		t.Errorf("RenderedStatus(code-review) = %v, want StatusBlocked: fork child 01a isn't done even though root 01 is", got)
	}
}

func TestEpic_RenderedStatus_CodeReviewTicketsDontBlockEachOther(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 9, Identifier: "09", Type: typeCodeReview, Status: "open"},
		{Number: 10, Identifier: "10", Type: typeCodeReview, Status: "open"},
		{Number: 1, Identifier: "01", Status: "done"},
	}}
	got9 := epic.RenderedStatus(epic.Tickets[0])
	if got9 != StatusOpen {
		t.Errorf("RenderedStatus(09) = %v, want StatusOpen: a code-review ticket must not wait on another code-review ticket", got9)
	}
	got10 := epic.RenderedStatus(epic.Tickets[1])
	if got10 != StatusOpen {
		t.Errorf("RenderedStatus(10) = %v, want StatusOpen: a code-review ticket must not wait on another code-review ticket", got10)
	}
}

// TestEpic_RenderedStatus_ForkChildBlockedOnParentsOwnDone covers the
// implicit parent-done edge (ticket 10): an open fork child is schedulable
// once its parent's own Status: is done, and blocked while the parent is
// still claimed — the deadlock-regression pair that demonstrates the edge
// exists without ever recursing into the parent's own fork subtree.
func TestEpic_RenderedStatus_ForkChildBlockedOnParentsOwnDone(t *testing.T) {
	parent := "01"
	doneEpic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "open", Parent: &parent},
	}}
	if got := doneEpic.RenderedStatus(doneEpic.Tickets[1]); got != StatusOpen {
		t.Errorf("RenderedStatus(01a) = %v, want StatusOpen once parent 01 is done", got)
	}

	claimedEpic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "claimed"},
		{Number: 1, Identifier: "01a", Status: "open", Parent: &parent},
	}}
	if got := claimedEpic.RenderedStatus(claimedEpic.Tickets[1]); got != StatusBlocked {
		t.Errorf("RenderedStatus(01a) = %v, want StatusBlocked while parent 01 is still claimed", got)
	}
}

// TestEpic_RenderedStatus_ForkChildHeldByParentParkState covers a parent
// stuck in either park status: a broken or question-blocked parent must not
// silently seed a child iteration.
func TestEpic_RenderedStatus_ForkChildHeldByParentParkState(t *testing.T) {
	parent := "01"
	for _, parkStatus := range []string{"needs-answer", "needs-repair"} {
		epic := Epic{Tickets: []Ticket{
			{Number: 1, Identifier: "01", Status: parkStatus},
			{Number: 1, Identifier: "01a", Status: "open", Parent: &parent},
		}}
		if got := epic.RenderedStatus(epic.Tickets[1]); got != StatusBlocked {
			t.Errorf("RenderedStatus(01a) = %v, want StatusBlocked while parent 01 is %s", got, parkStatus)
		}
	}
}

// TestEpic_RenderedStatus_ForkChildReleasedByCommitlessDoneParent ties this
// ticket to 08: a commitless close still leaves Status: done on the parent,
// so it releases the child same as any other done parent.
func TestEpic_RenderedStatus_ForkChildReleasedByCommitlessDoneParent(t *testing.T) {
	parent := "01"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done", Commitless: true},
		{Number: 1, Identifier: "01a", Status: "open", Parent: &parent},
	}}
	if got := epic.RenderedStatus(epic.Tickets[1]); got != StatusOpen {
		t.Errorf("RenderedStatus(01a) = %v, want StatusOpen: commitless-done parent still releases its child", got)
	}
}

// TestEpic_RenderedStatus_ForkChildParentEdgeIsNonRecursive covers the
// "non-recursive on the parent's own status" property directly: 01 is done
// and its own further-forked grandchild 01a is still open (making
// e.Blocking(01) true), but 01b — a sibling fork child whose Parent points
// at 01 itself — only checks 01's own Status:, not 01's whole subtree, so it
// is schedulable regardless of 01a.
func TestEpic_RenderedStatus_ForkChildParentEdgeIsNonRecursive(t *testing.T) {
	root := "01"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "open", Parent: &root},
		{Number: 1, Identifier: "01b", Status: "open", Parent: &root},
	}}
	if !epic.Blocking(epic.Tickets[0]) {
		t.Fatalf("Blocking(01) = false, want true: fork child 01a isn't done")
	}
	if got := epic.RenderedStatus(epic.Tickets[2]); got != StatusOpen {
		t.Errorf("RenderedStatus(01b) = %v, want StatusOpen: the parent edge checks 01's own status only, not its subtree", got)
	}
}

func TestEpic_UnresolvedBlockers_CodeReviewExpansionNotWrittenToTicket(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 9, Identifier: "09", Type: typeCodeReview, Status: "open"},
		{Number: 1, Identifier: "01", Status: "open"},
	}}
	reviewTicket := epic.Tickets[0]
	if got := epic.UnresolvedBlockers(reviewTicket); len(got) == 0 {
		t.Fatalf("UnresolvedBlockers(code-review) = empty, want the synthesized blocker for 01")
	}
	if len(reviewTicket.BlockedBy) != 0 {
		t.Errorf("reviewTicket.BlockedBy = %v, want unchanged (empty): the expansion must not be written back to the ticket", reviewTicket.BlockedBy)
	}
}
