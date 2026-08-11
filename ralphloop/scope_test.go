package ralphloop

import (
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestResolveRunScope_ValidSubsetOwnsMembership(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01"},
		{Number: 2, Identifier: "02"},
		{Number: 3, Identifier: "03"},
	}}

	scope, err := ResolveRunScope(epic, []string{"01", "03"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}
	if !scope.Contains(epic.Tickets[0], epic) {
		t.Errorf("scope.Contains(01) = false, want true")
	}
	if scope.Contains(epic.Tickets[1], epic) {
		t.Errorf("scope.Contains(02) = true, want false")
	}
	if !scope.Contains(epic.Tickets[2], epic) {
		t.Errorf("scope.Contains(03) = false, want true")
	}
}

func TestResolveRunScope_RejectsUnknownIdentifier(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01"},
	}}

	_, err := ResolveRunScope(epic, []string{"99"})
	if err == nil {
		t.Fatal("ResolveRunScope() error = nil, want unknown identifier error")
	}
	for _, want := range []string{"delivery", "99", "not found"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ResolveRunScope() error = %q, want it to contain %q", err, want)
		}
	}
}

func TestRunScope_DoneAndTotalCount_AreScopedNotEpicWide(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "open"},
		{Number: 3, Identifier: "03", Status: "done"},
	}}

	scope, err := ResolveRunScope(epic, []string{"01", "02"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}
	if got := scope.TotalCount(epic); got != 2 {
		t.Errorf("TotalCount() = %d, want 2 (epic-wide total is 3)", got)
	}
	if got := scope.DoneCount(epic); got != 1 {
		t.Errorf("DoneCount() = %d, want 1 (epic-wide done is 2)", got)
	}
}

// TestRunScope_DoneCount_WaitingForChildrenParentNotDone pins the fix for a
// scoped RunScope.DoneCount that used to read a ticket's raw Status: (via
// IsDone) instead of its RenderedStatus, so a scoped run could count a
// done-but-waiting-on-its-fork-subtree parent as done when the equivalent
// whole-epic scope (via Epic.DoneCount) would not — the exact "scoped and
// fresh runs report identical counts" split the ticket calls out.
func TestRunScope_DoneCount_WaitingForChildrenParentNotDone(t *testing.T) {
	parent := "01"
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "open", Parent: &parent},
		{Number: 2, Identifier: "02", Status: "done"},
	}}

	scope, err := ResolveRunScope(epic, []string{"01", "01a", "02"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}
	if got := scope.DoneCount(epic); got != 1 {
		t.Errorf("DoneCount() = %d, want 1 (01 is waiting on fork child 01a, only 02 is done)", got)
	}
	if got, want := scope.DoneCount(epic), epic.DoneCount(); got != want {
		t.Errorf("scoped DoneCount() = %d, want %d (whole-epic DoneCount, since scope covers every ticket)", got, want)
	}
}

func TestRunScope_DoneAndTotalCount_WholeEpicMatchesEpicMethods(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "open"},
	}}

	scope, err := ResolveRunScope(epic, nil)
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}
	if got := scope.TotalCount(epic); got != epic.TotalCount() {
		t.Errorf("TotalCount() = %d, want %d", got, epic.TotalCount())
	}
	if got := scope.DoneCount(epic); got != epic.DoneCount() {
		t.Errorf("DoneCount() = %d, want %d", got, epic.DoneCount())
	}
}

func TestResolveRunScope_RejectsDuplicateIdentifier(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01"},
	}}

	_, err := ResolveRunScope(epic, []string{"01", "01"})
	if err == nil {
		t.Fatal("ResolveRunScope() error = nil, want duplicate identifier error")
	}
	for _, want := range []string{"delivery", "01", "more than once"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ResolveRunScope() error = %q, want it to contain %q", err, want)
		}
	}
}

func TestRunScope_AllDoneForCompletedSubset(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "done"},
		{Number: 3, Identifier: "03", Status: "open"},
	}}
	scope, err := ResolveRunScope(epic, []string{"01", "02"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	if !scope.AllDone(epic) {
		t.Errorf("scope.AllDone() = false, want true for a fully done subset")
	}
}

// TestRunScope_AllDone_NeedsAnswerSubsetIsNotDone covers ticket 08's inversion
// at scope level: needs-answer used to count as terminal and let a subset run
// exit, and now must leave the run parked instead.
func TestRunScope_AllDone_NeedsAnswerSubsetIsNotDone(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "needs-answer"},
	}}
	scope, err := ResolveRunScope(epic, []string{"01", "02"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	if scope.AllDone(epic) {
		t.Errorf("scope.AllDone() = true, want false while ticket 02 needs answer")
	}
}

func TestRunScope_FrontierHonorsUnresolvedEpicDependency(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "open"},
		{Number: 2, Identifier: "02", Status: "open", BlockedBy: []string{"01"}},
	}}
	scope, err := ResolveRunScope(epic, []string{"02"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	if frontier := scope.Frontier(epic); len(frontier) != 0 {
		t.Errorf("scope.Frontier() = %v, want no runnable tickets", frontier)
	}
}

func TestRunScope_FrontierIncludesOnlySelectedTickets(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "open"},
		{Number: 2, Identifier: "02", Status: "open"},
		{Number: 3, Identifier: "03", Status: "open"},
	}}
	scope, err := ResolveRunScope(epic, []string{"03", "01"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	frontier := scope.Frontier(epic)
	if len(frontier) != 2 {
		t.Fatalf("len(scope.Frontier()) = %d, want 2", len(frontier))
	}
	if frontier[0].DisplayNumber() != "01" || frontier[1].DisplayNumber() != "03" {
		t.Errorf("scope.Frontier() identifiers = [%s, %s], want [01, 03]", frontier[0].DisplayNumber(), frontier[1].DisplayNumber())
	}
}

func TestRunScope_ContainsWalksParentChain(t *testing.T) {
	original := "03"
	child := "03b"
	grandchild := "03b2"
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01"},
		{Number: 3, Identifier: original},
		{Number: 3, Identifier: child, Parent: &original},
		{Number: 3, Identifier: grandchild, Parent: &child},
		{Number: 9, Identifier: "09"},
	}}
	scope, err := ResolveRunScope(epic, []string{"03"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	if !scope.Contains(epic.Tickets[2], epic) {
		t.Errorf("scope.Contains(03b) = false, want true (one Parent hop from 03)")
	}
	if !scope.Contains(epic.Tickets[3], epic) {
		t.Errorf("scope.Contains(03b2) = false, want true (two Parent hops from 03)")
	}
	if scope.Contains(epic.Tickets[4], epic) {
		t.Errorf("scope.Contains(09) = true, want false (no Parent chain to a requested ticket)")
	}
}

func TestRunScope_AllDone_DescendantTicketsDontTripSanityCheck(t *testing.T) {
	original := "01"
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01b", Parent: &original, Status: "done"},
		{Number: 1, Identifier: "01c", Parent: &original, Status: "done"},
	}}
	scope, err := ResolveRunScope(epic, []string{"01"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	if !scope.AllDone(epic) {
		t.Errorf("scope.AllDone() = false, want true once the requested ticket and its split descendants are all settled")
	}
}

func TestRunScope_AddMakesTicketImmediatelyClaimable(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "open"},
		{Number: 2, Identifier: "02", Status: "open"},
	}}
	scope, err := ResolveRunScope(epic, []string{"01"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}
	if scope.Contains(epic.Tickets[1], epic) {
		t.Fatalf("scope.Contains(02) = true before Add, want false")
	}

	scope.Add("02")

	if !scope.Contains(epic.Tickets[1], epic) {
		t.Errorf("scope.Contains(02) = false after Add, want true")
	}
	frontier := scope.Frontier(epic)
	if len(frontier) != 2 {
		t.Errorf("len(scope.Frontier()) = %d, want 2 after widening to include 02", len(frontier))
	}
}

func TestRunScope_AddOnDynamicScopeIsNoop(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "open"},
	}}
	scope, err := ResolveRunScope(epic, nil)
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	scope.Add("99") // must not panic

	if !scope.Contains(epic.Tickets[0], epic) {
		t.Errorf("scope.Contains(01) = false, want true (dynamic scope stays unrestricted)")
	}
}

func TestRunScope_AddIsRaceFreeAgainstConcurrentReads(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "open"},
		{Number: 2, Identifier: "02", Status: "open"},
	}}
	scope, err := ResolveRunScope(epic, []string{"01"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				scope.Frontier(epic)
				scope.Contains(epic.Tickets[1], epic)
			}
		}
	}()

	scope.Add("02")
	close(stop)
	wg.Wait()
}

func TestRunScope_UnsetRequestPreservesWholeEpicBehavior(t *testing.T) {
	initial := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
	}}
	scope, err := ResolveRunScope(initial, nil)
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	reloaded := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "open"},
	}}
	if !scope.Contains(reloaded.Tickets[1], reloaded) {
		t.Errorf("scope.Contains(new ticket) = false, want true for whole-epic scope")
	}
	if scope.AllDone(reloaded) {
		t.Errorf("scope.AllDone() = true, want false while new ticket is open")
	}
	frontier := scope.Frontier(reloaded)
	if len(frontier) != 1 || frontier[0].DisplayNumber() != "02" {
		t.Errorf("scope.Frontier() = %v, want ticket 02", frontier)
	}
}
