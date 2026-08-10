package tickets

import (
	"path/filepath"
	"strings"
	"testing"
)

func parentOf(id string) *string { return &id }

func ticketWithParent(number int, identifier, parent string) Ticket {
	t := Ticket{Number: number, Identifier: identifier, Status: "open"}
	if parent != "" {
		t.Parent = parentOf(parent)
	}
	return t
}

func TestValidateParentGraph_ValidGraph(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		ticketWithParent(1, "01", ""),
		ticketWithParent(1, "01a", "01"),
		ticketWithParent(1, "01a1", "01a"),
		ticketWithParent(2, "02", ""),
	}}

	if err := epic.ValidateParentGraph(); err != nil {
		t.Fatalf("ValidateParentGraph() = %v, want nil", err)
	}
}

func TestValidateParentGraph_DanglingParent(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		ticketWithParent(1, "01", ""),
		ticketWithParent(2, "02", "07"),
	}}

	err := epic.ValidateParentGraph()
	if err == nil {
		t.Fatal("ValidateParentGraph() = nil, want an error for a parent naming an absent ticket")
	}
	if !strings.Contains(err.Error(), "02") || !strings.Contains(err.Error(), "07") {
		t.Errorf("error = %q, want it to name both the ticket and its missing parent", err)
	}
}

func TestValidateParentGraph_Cycle(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		ticketWithParent(1, "01", "03"),
		ticketWithParent(2, "02", "01"),
		ticketWithParent(3, "03", "02"),
	}}

	err := epic.ValidateParentGraph()
	if err == nil {
		t.Fatal("ValidateParentGraph() = nil, want an error for a cyclic parent graph")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q, want it to report a cycle", err)
	}
	for _, id := range []string{"01", "02", "03"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("error = %q, want every edge in the cycle reported (missing %s)", err, id)
		}
	}
}

func TestValidateParentGraph_SelfParent(t *testing.T) {
	epic := Epic{Tickets: []Ticket{ticketWithParent(1, "01", "01")}}

	if err := epic.ValidateParentGraph(); err == nil {
		t.Fatal("ValidateParentGraph() = nil, want an error for a self-parent")
	}
}

func TestQuarantineInvalidParents_DropsEdgeAndFlagsTicket(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		ticketWithParent(1, "01", ""),
		ticketWithParent(2, "02", "01"),
		ticketWithParent(3, "03", "09"),
	}}

	epic.quarantineInvalidParents()

	if err := epic.ValidateParentGraph(); err != nil {
		t.Fatalf("graph still invalid after quarantine: %v", err)
	}
	if epic.Tickets[2].Parent != nil {
		t.Errorf("ticket 03 kept its dangling parent %q", *epic.Tickets[2].Parent)
	}
	if epic.Tickets[2].GraphErr == "" {
		t.Error("ticket 03 has no GraphErr recorded")
	}
	if got := epic.RenderedStatus(epic.Tickets[2]); got != StatusError {
		t.Errorf("RenderedStatus(03) = %v, want StatusError", got)
	}
	if epic.Tickets[1].GraphErr != "" || epic.Tickets[1].Parent == nil {
		t.Error("ticket 02's valid parent edge was disturbed")
	}
}

func TestLoad_CyclicParentGraphIsNeverExposed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "epic", "issues", "01-a.md"),
		"---\nid: \"01\"\nstatus: open\ntype: task\nparent: \"02\"\n---\nBody.\n")
	writeFile(t, filepath.Join(dir, "epic", "issues", "02-b.md"),
		"---\nid: \"02\"\nstatus: open\ntype: task\nparent: \"01\"\n---\nBody.\n")

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	epic := epics[0]
	if err := epic.ValidateParentGraph(); err != nil {
		t.Fatalf("Load handed out a cyclic parent graph: %v", err)
	}
	for _, ticket := range epic.Tickets {
		if ticket.GraphErr == "" {
			t.Errorf("ticket %s: no GraphErr recorded for its cycle edge", ticket.DisplayNumber())
		}
		if got := epic.RenderedStatus(ticket); got != StatusError {
			t.Errorf("ticket %s renders as %v, want StatusError", ticket.DisplayNumber(), got)
		}
	}
}

func TestLoad_DanglingParentIsNeverExposed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "epic", "issues", "01-a.md"),
		"---\nid: \"01\"\nstatus: open\ntype: task\nparent: \"09\"\n---\nBody.\n")

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := epics[0].ValidateParentGraph(); err != nil {
		t.Fatalf("Load handed out a dangling parent edge: %v", err)
	}
	if epics[0].Tickets[0].GraphErr == "" {
		t.Error("no GraphErr recorded for the dangling parent")
	}
}

func TestForkParents_ResolvesTokenRegardlessOfPadding(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		ticketWithParent(4, "04", ""),
		ticketWithParent(4, "04a", "4"),
		ticketWithParent(4, "04b", "04"),
		ticketWithParent(4, "04a1", "04A"),
	}}
	parents := epic.ForkParents()

	for _, tc := range []struct {
		child string
		want  string
	}{
		{child: "04a", want: "04"},
		{child: "04b", want: "04"},
		{child: "04a1", want: "04a"},
	} {
		child := epic.Tickets[indexOfIdentifier(t, epic, tc.child)]
		parent, ok := parents.Of(child)
		if !ok {
			t.Fatalf("ForkParents().Of(%s) reported no parent, want %s", tc.child, tc.want)
		}
		if parent.Identifier != tc.want {
			t.Errorf("ForkParents().Of(%s) = %s, want %s", tc.child, parent.Identifier, tc.want)
		}
	}
}

func TestForkParents_AbsentForRootAndUnknownToken(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		ticketWithParent(1, "01", ""),
		ticketWithParent(2, "02", "09"),
	}}
	parents := epic.ForkParents()

	if _, ok := parents.Of(epic.Tickets[0]); ok {
		t.Error("ForkParents().Of(01) reported a parent for a ticket with no parent token")
	}
	if _, ok := parents.Of(epic.Tickets[1]); ok {
		t.Error("ForkParents().Of(02) reported a parent for a token naming no sibling")
	}
}

func indexOfIdentifier(t *testing.T, epic Epic, identifier string) int {
	t.Helper()
	for i, ticket := range epic.Tickets {
		if ticket.Identifier == identifier {
			return i
		}
	}
	t.Fatalf("no ticket %s in fixture", identifier)
	return -1
}

func TestQuarantineInvalidParents_BreaksCycle(t *testing.T) {
	epic := Epic{Tickets: []Ticket{
		ticketWithParent(1, "01", "02"),
		ticketWithParent(2, "02", "01"),
	}}

	epic.quarantineInvalidParents()

	if err := epic.ValidateParentGraph(); err != nil {
		t.Fatalf("graph still cyclic after quarantine: %v", err)
	}
}
