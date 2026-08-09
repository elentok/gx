package tickets

import (
	"testing"
	"time"
)

func strPtr(s string) *string { return &s }

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
// rather than being one of 06's lettered fork replacements. Its own
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

// TestEpic_UnresolvedBlockers_SelfBlockedResolves covers a ticket
// pathologically Blocked by: its own identifier — it must resolve rather
// than deadlock, since it can never become done while still being checked.
func TestEpic_UnresolvedBlockers_SelfBlockedResolves(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", BlockedBy: []string{"1"}, Status: "open"},
	}}
	if got := epic.UnresolvedBlockers(epic.Tickets[0]); got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil for a ticket blocked by its own identifier", got)
	}
}

// TestEpic_UnresolvedBlockers_LetteredForkRequiresAllSiblingsDone covers a
// mid-flight fork: 03's Children (03a, 03b) are now a direct, walkable edge
// (see Epic.FullyDone) rather than inferred from shared numbering. The
// original (03) is closed as done+commitless immediately at fork time, well
// before its replacements (03a, 03b) land, so a blocker on the bare number
// "3" must stay unresolved until every child recorded in 03's Children is
// done too, not just 03 itself.
func TestEpic_UnresolvedBlockers_LetteredForkRequiresAllSiblingsDone(t *testing.T) {
	original := "03"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"3"}},
		{Number: 3, Identifier: "03", Status: "done", Commitless: true, Children: []string{"03a", "03b"}}, // original, closed at fork time
		{Number: 3, Identifier: "03a", Status: "done", Parent: &original},
		{Number: 3, Identifier: "03b", Status: "open", Parent: &original}, // still in flight
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	if len(got) != 1 || got[0] != "3" {
		t.Fatalf("UnresolvedBlockers = %v, want [3] while 03b is still open", got)
	}

	epic.Tickets[3].Status = "done"
	got = epic.UnresolvedBlockers(epic.Tickets[0])
	if got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil once every child in 03's Children is done", got)
	}
}

// TestEpic_UnresolvedBlockers_LetteredTokenNamesOneSibling covers the
// opposite of the bare-number case above: "Blocked by: 03a" names one
// specific fork sibling, so it resolves as soon as that ticket alone is
// done — it must not require its still-open siblings (03b) or the
// original (03) to finish too, unlike a bare "Blocked by: 3".
func TestEpic_UnresolvedBlockers_LetteredTokenNamesOneSibling(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"3a"}}, // no zero-padding, unlike Identifier "03a" below
		{Number: 3, Identifier: "03", Status: "done", Commitless: true},
		{Number: 3, Identifier: "03a", Status: "done"},
		{Number: 3, Identifier: "03b", Status: "open"}, // still in flight, doesn't matter
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[0])
	if got != nil {
		t.Errorf("UnresolvedBlockers = %v, want nil — 03a alone is done", got)
	}
}

// TestEpic_UnresolvedBlockers_ForkSiblingsDontBlockEachOther covers 05's
// real-world fork: both 05b and 05c inherit "Blocked by: 05" from the
// fork, and share Number 5 with each other. Without excluding fork
// siblings from the family count, each would need the other done too,
// deadlocking them against each other despite 05 itself being done.
func TestEpic_UnresolvedBlockers_ForkSiblingsDontBlockEachOther(t *testing.T) {
	original := "05"
	epic := Epic{Tickets: []Ticket{
		{Number: 5, Identifier: "05", Status: "done", Children: []string{"05b", "05c"}},
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

// TestEpic_UnresolvedBlockers_InheritedTokenNotBlockedByOwnDescendant covers
// a sequential fork: 01 forks into 01a, which itself later forks into
// 01b (01b's Parent is 01a, not 01, and 01b is blocked_by 01a). 01 lists
// both 01a and 01b as Children (gx-investigate's drain-queue/
// tickets-tree finding: the root ticket's children can legitimately name a
// grandchild this way). 01a's own inherited "Blocked by: 01" must not
// require 01b — its own child — to be done first: 01b can't even start
// before 01a does, so that would deadlock 01a against its own not-yet-begun
// follow-on work.
func TestEpic_UnresolvedBlockers_InheritedTokenNotBlockedByOwnDescendant(t *testing.T) {
	root := "01"
	mechanism := "01a"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done", Commitless: true, Children: []string{"01a", "01b"}},
		{Number: 1, Identifier: "01a", BlockedBy: []string{"01"}, Parent: &root, Status: "ready-for-agent"},
		{Number: 1, Identifier: "01b", BlockedBy: []string{"01a"}, Parent: &mechanism, Status: "open"},
	}}
	if got := epic.UnresolvedBlockers(epic.Tickets[1]); got != nil {
		t.Errorf("UnresolvedBlockers(01a) = %v, want nil since 01 is done and 01b is 01a's own not-yet-started child", got)
	}
	if status := epic.RenderedStatus(epic.Tickets[1]); status != StatusOpen {
		t.Errorf("RenderedStatus(01a) = %v, want StatusOpen (ready-for-agent, unblocked)", status)
	}
	// 01b itself still correctly waits for 01a, its real direct blocker.
	if got := epic.UnresolvedBlockers(epic.Tickets[2]); len(got) != 1 || got[0] != "01a" {
		t.Errorf("UnresolvedBlockers(01b) = %v, want [01a] since 01a isn't done yet", got)
	}
}

// TestEpic_UnresolvedBlockers_DirectSiblingTokenIsEnforced covers the
// opposite of ForkSiblingsDontBlockEachOther above: 02c's "Blocked by: 02b"
// names its fork sibling 02b directly rather than their shared parent, so
// isForkSibling's shared-Parent exclusion (needed for the inherited-token
// case, see unresolvedBlockers) must not apply here — 02b's real status has
// to be checked.
func TestEpic_UnresolvedBlockers_DirectSiblingTokenIsEnforced(t *testing.T) {
	original := "02"
	epic := Epic{Tickets: []Ticket{
		{Number: 2, Identifier: "02", Status: "done", Commitless: true, Children: []string{"02b", "02c"}},
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

// TestEpic_UnresolvedBlockers_TransitiveThroughPrematurelyDoneForkPlaceholder
// covers the tickets-tree bug: "06" forks into "06b" (Blocked by: 06) and
// "06c" (Blocked by: 06b), both commitless placeholders born status: done at
// fork time — before their own fork chains ("06b"->"06b1", "06c"->"06c1")
// ever run. "06c1" (Blocked by: 06c) must stay blocked while "06b"'s own
// declared blocker "06b1" is still open, even though "06c" itself already
// reads as done — fullyDone must check a candidate blocker's own Blocked by:
// list, not just its status field, or this false-unblocks immediately.
func TestEpic_UnresolvedBlockers_TransitiveThroughPrematurelyDoneForkPlaceholder(t *testing.T) {
	root := "06"
	b := "06b"
	c := "06c"
	epic := Epic{Tickets: []Ticket{
		{Number: 6, Identifier: "06", Status: "done", Children: []string{"06b", "06c"}},
		{Number: 6, Identifier: "06b", Parent: &root, BlockedBy: []string{"06"}, Status: "done", Commitless: true, Children: []string{"06b1"}},
		{Number: 6, Identifier: "06b1", Parent: &b, BlockedBy: []string{"06b"}, Status: "open"},
		{Number: 6, Identifier: "06c", Parent: &root, BlockedBy: []string{"06b"}, Status: "done", Commitless: true, Children: []string{"06c1"}},
		{Number: 6, Identifier: "06c1", Parent: &c, BlockedBy: []string{"06c"}, Status: "open"},
	}}
	c1 := epic.Tickets[4]
	got := epic.UnresolvedBlockers(c1)
	if len(got) != 1 || got[0] != "06c" {
		t.Fatalf("UnresolvedBlockers(06c1) = %v, want [06c] while 06b1 (06c's own blocker's child) is still open", got)
	}
	if status := epic.RenderedStatus(c1); status != StatusBlocked {
		t.Errorf("RenderedStatus(06c1) = %v, want StatusBlocked", status)
	}

	epic.Tickets[2].Status = "done" // 06b1 finishes
	got = epic.UnresolvedBlockers(c1)
	if got != nil {
		t.Errorf("UnresolvedBlockers(06c1) = %v, want nil once 06b1 is done", got)
	}
}

// TestEpic_UnresolvedBlockers_MisparentedGrandchildStillEnforced covers the
// gotchas.md-documented mistake live in tickets-tree four times: a ticket's
// own Parent gets written as the fork root instead of its real direct
// parent. 05b1 really is 05b's child (05b.Children lists it correctly), but
// 05b1.Parent is mis-written as "05" — the same value 05c.Parent carries.
// Before isForkSibling's exclusion was made conditional on an inherited
// token, this made 05b1 look like 05c's own fork sibling and skipped it
// entirely, so 05c (Blocked by: 05b, a direct target, not inherited) read
// as unblocked while 05b1 was still open.
func TestEpic_UnresolvedBlockers_MisparentedGrandchildStillEnforced(t *testing.T) {
	root := "05"
	epic := Epic{Tickets: []Ticket{
		{Number: 5, Identifier: "05", Status: "done", Commitless: true, Children: []string{"05b", "05c"}},
		{Number: 5, Identifier: "05b", Parent: &root, Status: "done", Commitless: true, Children: []string{"05b1"}},
		{Number: 5, Identifier: "05b1", Parent: &root, Status: "open"}, // mis-parented: should be "05b"
		{Number: 5, Identifier: "05c", Parent: &root, BlockedBy: []string{"05b"}, Status: "open"},
	}}
	c := epic.Tickets[3]
	got := epic.UnresolvedBlockers(c)
	if len(got) != 1 || got[0] != "05b" {
		t.Fatalf("UnresolvedBlockers(05c) = %v, want [05b]: 05b1 (05b's real child, mis-parented to root) is still open", got)
	}

	epic.Tickets[2].Status = "done" // 05b1 finishes
	got = epic.UnresolvedBlockers(c)
	if got != nil {
		t.Errorf("UnresolvedBlockers(05c) = %v, want nil once 05b and 05b1 are both done", got)
	}
}

// TestEpic_UnresolvedBlockers_MisparentedSelfDoesNotDeadlockOnOwnBlocker
// covers the flip side of the mis-parenting above: 05c's own Parent is
// mis-written as its direct blocker "05b" instead of the real fork root
// "05". Deriving descendants from Parent (see childrenIndex) would
// otherwise make 05c reachable as its own blocker's descendant — without
// isSelfOrDescendant's unconditional (not inherited-token-gated) exclusion,
// resolving 05c's own blocker would require 05c itself to be done first: a
// permanent deadlock, since 05c can't finish before its own blocker
// resolves.
func TestEpic_UnresolvedBlockers_MisparentedSelfDoesNotDeadlockOnOwnBlocker(t *testing.T) {
	blocker := "05b"
	epic := Epic{Tickets: []Ticket{
		{Number: 5, Identifier: "05", Status: "done"},
		{Number: 5, Identifier: "05b", Status: "done"},
		{Number: 5, Identifier: "05c", Parent: &blocker, BlockedBy: []string{"05b"}, Status: "open"},
	}}
	c := epic.Tickets[2]
	if got := epic.UnresolvedBlockers(c); got != nil {
		t.Fatalf("UnresolvedBlockers(05c) = %v, want nil: 05b is done and 05c must be excluded from its own blocker's subtree check", got)
	}
}

// TestEpic_UnresolvedBlockers_SharedOpenDescendantCheckedIndependently covers
// the visiting-memo hazard fullyDone's defer-delete fix closes: "20d" is a
// real, still-open child of two different blockers ("20p".Children and
// "20q".Children both list it — a duplicate-bookkeeping slip, not a cycle).
// A permanent (never-unwound) visiting flag would set visiting["20d"] = true
// the instant fullyDone first descends into it while resolving "20t"'s first
// token ("20p") — before even checking whether "20d" is actually done — and
// then wrongly report "20d" as done when it's reached again resolving the
// second token ("20q"), since visiting only guards the current recursion
// path, not the whole call.
func TestEpic_UnresolvedBlockers_SharedOpenDescendantCheckedIndependently(t *testing.T) {
	root := "20"
	epic := Epic{Tickets: []Ticket{
		{Number: 20, Identifier: "20", Status: "done", Children: []string{"20p", "20q"}},
		{Number: 20, Identifier: "20p", Parent: &root, Status: "done", Children: []string{"20d"}},
		{Number: 20, Identifier: "20q", Parent: &root, Status: "done", Children: []string{"20d"}},
		{Number: 20, Identifier: "20d", Parent: &root, Status: "open"},
		{Number: 20, Identifier: "20t", Parent: &root, BlockedBy: []string{"20p", "20q"}, Status: "open"},
	}}
	got := epic.UnresolvedBlockers(epic.Tickets[4])
	if len(got) != 2 || got[0] != "20p" || got[1] != "20q" {
		t.Fatalf("UnresolvedBlockers(20t) = %v, want [20p 20q]: 20d (their shared open child) must be re-checked resolving each token, not memoized true after the first", got)
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
// mid-flight fork blocker (see Epic.FullyDone): "Blocked by: 3" should
// surface every one of 03's not-yet-done Children, not just the original, so
// confirming the modal adds them all to the checked set.
func TestEpic_BlockingTickets_BareNumberResolvesEveryNotYetDoneSibling(t *testing.T) {
	original := "03"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, BlockedBy: []string{"3"}},
		{Number: 3, Identifier: "03", Title: "Original", Status: "done", Children: []string{"03a", "03b"}},
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
		{Number: 3, Identifier: "03", Status: "done", Commitless: true},
		{Number: 3, Identifier: "03a", Title: "Split A", Status: "open"},
		{Number: 3, Identifier: "03b", Title: "Split B", Status: "open"},
	}}
	got := epic.BlockingTickets(epic.Tickets[0])
	if len(got) != 1 || got[0].Identifier != "03a" {
		t.Fatalf("BlockingTickets = %+v, want [03a Split A]", got)
	}
}

func TestEpic_FullyDone_ChildlessDoneTicketIsFullyDone(t *testing.T) {
	epic := Epic{Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "done"}}}
	if !epic.FullyDone(epic.Tickets[0]) {
		t.Errorf("FullyDone = false, want true for a childless done ticket")
	}
}

func TestEpic_FullyDone_NotDoneItselfIsNotFullyDone(t *testing.T) {
	epic := Epic{Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open"}}}
	if epic.FullyDone(epic.Tickets[0]) {
		t.Errorf("FullyDone = true, want false for a ticket whose own status isn't done")
	}
}

func TestEpic_FullyDone_DoneWithUndoneChildIsNotFullyDone(t *testing.T) {
	parent := "01"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done", Children: []string{"01a"}},
		{Number: 1, Identifier: "01a", Status: "open", Parent: &parent},
	}}
	if epic.FullyDone(epic.Tickets[0]) {
		t.Errorf("FullyDone = true, want false: child 01a isn't done")
	}
}

func TestEpic_FullyDone_DoneWithUndoneGrandchildIsNotFullyDone(t *testing.T) {
	parent := "01"
	child := "01a"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done", Children: []string{"01a"}},
		{Number: 1, Identifier: "01a", Status: "done", Parent: &parent, Children: []string{"01a-01"}},
		{Number: 1, Identifier: "01a-01", Status: "open", Parent: &child},
	}}
	if epic.FullyDone(epic.Tickets[0]) {
		t.Errorf("FullyDone = true, want false: grandchild 01a-01 isn't done despite 01 and 01a both being done")
	}
}

func TestEpic_FullyDone_DoneWithFullyDoneChildIsFullyDone(t *testing.T) {
	parent := "01"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done", Children: []string{"01a"}},
		{Number: 1, Identifier: "01a", Status: "done", Parent: &parent},
	}}
	if !epic.FullyDone(epic.Tickets[0]) {
		t.Errorf("FullyDone = false, want true: 01 and its only child 01a are both done")
	}
}

// TestEpic_FullyDone_CycleTerminates guards against a malformed
// Children/Parent loop (e.g. hand-edited frontmatter) hanging or
// stack-overflowing FullyDone's recursion — it must terminate one way or
// another, regardless of which boolean it settles on.
func TestEpic_FullyDone_CycleTerminates(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done", Children: []string{"02"}},
		{Number: 2, Identifier: "02", Status: "done", Children: []string{"01"}},
	}}
	done := make(chan bool, 1)
	go func() { done <- epic.FullyDone(epic.Tickets[0]) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("FullyDone did not terminate on a Children/Parent cycle")
	}
}

// TestIsSelfOrDescendant exercises the unconditional half of fullyDone's
// exclude predicate on its own — self, direct child, and grandchild must
// always be excluded regardless of which kind of Blocked by: token is being
// resolved; an ancestor or an unrelated ticket must not be.
func TestIsSelfOrDescendant(t *testing.T) {
	root := "01"
	mechanism := "01a"
	notification := "01b"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done", Children: []string{"01a", "01b"}},
		{Number: 1, Identifier: "01a", Parent: &root, Status: "ready-for-agent"},
		{Number: 1, Identifier: "01b", Parent: &mechanism, Status: "open"},     // child of 01a
		{Number: 1, Identifier: "01c", Parent: &root, Status: "open"},          // true sibling of 01a, NOT a descendant
		{Number: 1, Identifier: "01b1", Parent: &notification, Status: "open"}, // grandchild, via 01b
		{Number: 2, Identifier: "02", Status: "open"},                          // unrelated ticket
	}}
	byID := ticketsByIdentifier(epic)
	byNumberAndSuffix := epic.byNumberAndSuffix()
	t01a := byID["01a"]

	cases := []struct {
		name  string
		other Ticket
		want  bool
	}{
		{"self", t01a, true},
		{"direct child", byID["01b"], true},
		{"grandchild reached via child", byID["01b1"], true},
		{"fork sibling (shared Parent, not a descendant)", byID["01c"], false},
		{"own ancestor (root)", byID["01"], false},
		{"unrelated ticket, no Parent link at all", byID["02"], false},
	}
	for _, c := range cases {
		if got := isSelfOrDescendant(t01a, c.other, byNumberAndSuffix); got != c.want {
			t.Errorf("isSelfOrDescendant(01a, %s) = %v, want %v", c.other.Identifier, got, c.want)
		}
	}
}

// TestIsForkSibling exercises the conditional half of fullyDone's exclude
// predicate: two tickets sharing a Parent are fork siblings regardless of
// descent, but this alone must never decide exclusion — unresolvedBlockers
// only applies it when the token being resolved is inherited from an
// ancestor (see TestEpic_UnresolvedBlockers_DirectSiblingTokenIsEnforced and
// TestEpic_UnresolvedBlockers_InheritedTokenExcludesDirectForkSibling below).
func TestIsForkSibling(t *testing.T) {
	root := "01"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Parent: &root, Status: "open"},
		{Number: 1, Identifier: "01b", Parent: &root, Status: "open"},
		{Number: 2, Identifier: "02", Status: "open"},
	}}
	byID := ticketsByIdentifier(epic)

	if !isForkSibling(byID["01a"], byID["01b"]) {
		t.Error("isForkSibling(01a, 01b) = false, want true: both forked from 01")
	}
	if isForkSibling(byID["01a"], byID["01"]) {
		t.Error("isForkSibling(01a, 01) = true, want false: 01 is 01a's Parent, not a shared-Parent sibling")
	}
	if isForkSibling(byID["01a"], byID["02"]) {
		t.Error("isForkSibling(01a, 02) = true, want false: 02 has no Parent at all")
	}
}

// TestIsDescendantOf covers isDescendantOf on its own — the Parent-chain
// walk isSelfOrForkSiblingOrDescendant layers the sibling check on top of.
func TestIsDescendantOf(t *testing.T) {
	root := "01"
	mid := "01a"
	epic := Epic{Tickets: []Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Parent: &root, Status: "done"},
		{Number: 1, Identifier: "01b", Parent: &mid, Status: "open"},
		{Number: 2, Identifier: "02", Status: "open"},
		{Number: 3, Identifier: "03", Parent: strPtr("no-such-ticket"), Status: "open"},
	}}
	byID := ticketsByIdentifier(epic)
	byNumberAndSuffix := epic.byNumberAndSuffix()
	t01 := byID["01"]
	t01a := byID["01a"]

	if !isDescendantOf(t01, byID["01a"], byNumberAndSuffix) {
		t.Error("isDescendantOf(01, 01a) = false, want true: 01a's Parent is 01")
	}
	if !isDescendantOf(t01, byID["01b"], byNumberAndSuffix) {
		t.Error("isDescendantOf(01, 01b) = false, want true: 01b descends from 01 via 01a")
	}
	if isDescendantOf(t01a, byID["01"], byNumberAndSuffix) {
		t.Error("isDescendantOf(01a, 01) = true, want false: 01 is 01a's ancestor, not its descendant")
	}
	if isDescendantOf(t01, byID["02"], byNumberAndSuffix) {
		t.Error("isDescendantOf(01, 02) = true, want false: 02 has no Parent at all")
	}
	if isDescendantOf(t01, byID["03"], byNumberAndSuffix) {
		t.Error("isDescendantOf(01, 03) = true, want false: 03's Parent token doesn't resolve to any ticket")
	}
}

// ticketsByIdentifier indexes epic's tickets by their literal Identifier
// string, for tests that want to grab a specific fixture ticket by name
// without recomputing byNumberAndSuffix's zero-padding-insensitive key.
func ticketsByIdentifier(epic Epic) map[string]Ticket {
	byID := make(map[string]Ticket, len(epic.Tickets))
	for _, ticket := range epic.Tickets {
		byID[ticket.Identifier] = ticket
	}
	return byID
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
