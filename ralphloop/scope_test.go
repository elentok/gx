package ralphloop

import (
	"strings"
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
	if !scope.Contains(epic.Tickets[0]) {
		t.Errorf("scope.Contains(01) = false, want true")
	}
	if scope.Contains(epic.Tickets[1]) {
		t.Errorf("scope.Contains(02) = true, want false")
	}
	if !scope.Contains(epic.Tickets[2]) {
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

func TestRunScope_AllSettledForCompletedSubset(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "needs-info"},
		{Number: 3, Identifier: "03", Status: "open"},
	}}
	scope, err := ResolveRunScope(epic, []string{"01", "02"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	if !scope.AllSettled(epic) {
		t.Errorf("scope.AllSettled() = false, want true for terminal subset")
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
	if !scope.Contains(reloaded.Tickets[1]) {
		t.Errorf("scope.Contains(new ticket) = false, want true for whole-epic scope")
	}
	if scope.AllSettled(reloaded) {
		t.Errorf("scope.AllSettled() = true, want false while new ticket is open")
	}
	frontier := scope.Frontier(reloaded)
	if len(frontier) != 1 || frontier[0].DisplayNumber() != "02" {
		t.Errorf("scope.Frontier() = %v, want ticket 02", frontier)
	}
}
