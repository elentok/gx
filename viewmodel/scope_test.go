package viewmodel_test

import (
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/viewmodel"
)

func scopeSnapshot() server.Snapshot {
	return server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "alpha:e/01"},
		{Address: "beta:e/01"},
	}}
}

func addresses(ts []server.TicketInfo) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Address)
	}
	return out
}

func TestScopedTickets_DefaultsToCwdProjectAndTogglesToAll(t *testing.T) {
	vm := viewmodel.State{}.ApplySnapshot(scopeSnapshot())
	scope := viewmodel.Scope{CwdProject: "alpha"}

	if got := addresses(vm.ScopedTickets(scope)); len(got) != 1 || got[0] != "alpha:e/01" {
		t.Fatalf("scoped = %v, want only alpha", got)
	}
	if got := addresses(vm.ScopedTickets(scope.Toggle())); len(got) != 2 {
		t.Fatalf("all = %v, want both projects", got)
	}
	if scope.UnregisteredHint() != "" {
		t.Fatal("registered project must not show the hint")
	}
}

func TestScopedTickets_OutsideProjectShowsAllWithHint(t *testing.T) {
	vm := viewmodel.State{}.ApplySnapshot(scopeSnapshot())
	scope := viewmodel.Scope{}

	if got := addresses(vm.ScopedTickets(scope)); len(got) != 2 {
		t.Fatalf("scoped = %v, want all", got)
	}
	if scope.UnregisteredHint() == "" {
		t.Fatal("expected the unregistered hint")
	}
}
