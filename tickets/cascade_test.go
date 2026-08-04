package tickets

import "testing"

func hasPath(tickets []Ticket, path string) bool {
	for _, t := range tickets {
		if t.Path == path {
			return true
		}
	}
	return false
}

func TestEpic_CascadeDelete_NoDependentsDeletesJustTarget(t *testing.T) {
	target := Ticket{Number: 1, Identifier: "01", Path: "01", Status: "open"}
	other := Ticket{Number: 2, Identifier: "02", Path: "02", Status: "open"}
	epic := Epic{Tickets: []Ticket{target, other}}

	toDelete, toClear := epic.CascadeDelete(target)

	if len(toDelete) != 1 || toDelete[0].Path != "01" {
		t.Fatalf("toDelete = %+v, want just the target", toDelete)
	}
	if len(toClear) != 0 {
		t.Fatalf("toClear = %+v, want none", toClear)
	}
}

func TestEpic_CascadeDelete_TransitiveDependentsAllDeleted(t *testing.T) {
	root := Ticket{Number: 1, Identifier: "01", Path: "01", Status: "open"}
	mid := Ticket{Number: 2, Identifier: "02", Path: "02", Status: "open", BlockedBy: []string{"01"}}
	leaf := Ticket{Number: 3, Identifier: "03", Path: "03", Status: "open", BlockedBy: []string{"02"}}
	unrelated := Ticket{Number: 4, Identifier: "04", Path: "04", Status: "open"}
	epic := Epic{Tickets: []Ticket{root, mid, leaf, unrelated}}

	toDelete, toClear := epic.CascadeDelete(root)

	if len(toDelete) != 3 || !hasPath(toDelete, "01") || !hasPath(toDelete, "02") || !hasPath(toDelete, "03") {
		t.Fatalf("toDelete = %+v, want root+mid+leaf", toDelete)
	}
	if hasPath(toDelete, "04") {
		t.Fatalf("toDelete = %+v, unrelated ticket should not be included", toDelete)
	}
	if len(toClear) != 0 {
		t.Fatalf("toClear = %+v, want none", toClear)
	}
}

func TestEpic_CascadeDelete_StopsAtDoneTicketAndClearsItsDanglingEntry(t *testing.T) {
	root := Ticket{Number: 1, Identifier: "01", Path: "01", Status: "open"}
	doneDependent := Ticket{Number: 2, Identifier: "02", Path: "02", Status: "done", BlockedBy: []string{"01"}}
	// Blocked by the done ticket, not by root directly — must survive
	// untouched since traversal stops at the done ticket.
	behindDone := Ticket{Number: 3, Identifier: "03", Path: "03", Status: "open", BlockedBy: []string{"02"}}
	epic := Epic{Tickets: []Ticket{root, doneDependent, behindDone}}

	toDelete, toClear := epic.CascadeDelete(root)

	if len(toDelete) != 1 || toDelete[0].Path != "01" {
		t.Fatalf("toDelete = %+v, want just root", toDelete)
	}
	if len(toClear) != 1 || toClear[0].Path != "02" {
		t.Fatalf("toClear = %+v, want the done dependent", toClear)
	}
}

func TestEpic_CascadeDelete_LetteredTokenMatchesOnlyThatSibling(t *testing.T) {
	target := Ticket{Number: 4, Identifier: "04a", Path: "04a", Status: "open"}
	sibling := Ticket{Number: 4, Identifier: "04b", Path: "04b", Status: "open"}
	// Blocked by 04a specifically, not the whole family.
	dependent := Ticket{Number: 5, Identifier: "05", Path: "05", Status: "open", BlockedBy: []string{"04a"}}
	epic := Epic{Tickets: []Ticket{target, sibling, dependent}}

	toDelete, _ := epic.CascadeDelete(target)

	if len(toDelete) != 2 || !hasPath(toDelete, "04a") || !hasPath(toDelete, "05") {
		t.Fatalf("toDelete = %+v, want target+dependent only", toDelete)
	}
}
